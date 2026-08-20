package parity

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

type ProtocolMode string

const (
	ProtocolChatCompletions ProtocolMode = "chat_completions"
	ProtocolResponses       ProtocolMode = "responses"
)

type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

type StreamResult struct {
	Text           string
	ToolCalls      []ToolCall
	TerminalFrames int
	RepairedOutput bool
}

type ProtocolError struct {
	Code       string
	StatusCode int
	Message    string
	Cause      error
}

func (e *ProtocolError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

func (e *ProtocolError) Unwrap() error { return e.Cause }

func protocolError(code, message string, cause error) *ProtocolError {
	return &ProtocolError{Code: code, Message: message, Cause: cause}
}

// ProtocolErrorFromHTTP maps upstream failures without retaining a potentially
// sensitive response body.
func ProtocolErrorFromHTTP(status int, _ []byte) *ProtocolError {
	code := "upstream_http_error"
	switch {
	case status == http.StatusNotFound:
		code = "upstream_route_unavailable"
	case status == http.StatusTooManyRequests:
		code = "upstream_rate_limited"
	case status >= 500:
		code = "upstream_server_error"
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		code = "upstream_auth_error"
	}
	return &ProtocolError{
		Code:       code,
		StatusCode: status,
		Message:    fmt.Sprintf("AI upstream returned HTTP %d", status),
	}
}

type toolCallBuilder struct {
	index     int
	id        string
	name      string
	arguments strings.Builder
	complete  bool
}

// NormalizeSSE converts Chat Completions or Responses SSE into one stable
// result and rejects ambiguous/incomplete streams.
func NormalizeSSE(mode ProtocolMode, reader io.Reader) (StreamResult, error) {
	result := StreamResult{}
	chatCalls := map[int]*toolCallBuilder{}
	responseCalls := map[string]*toolCallBuilder{}
	responseOrder := []string{}
	responseCompleted := false
	completedOutputCalls := 0

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") || strings.HasPrefix(line, "id:") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			if result.TerminalFrames == 0 {
				result.TerminalFrames = 1
			}
			continue
		}

		var raw map[string]any
		if err := json.Unmarshal([]byte(payload), &raw); err != nil {
			return StreamResult{}, protocolError("malformed_sse_json", "AI stream contains malformed JSON", err)
		}

		switch mode {
		case ProtocolChatCompletions:
			if err := consumeChatChunk(raw, &result, chatCalls); err != nil {
				return StreamResult{}, err
			}
		case ProtocolResponses:
			completed, outputCalls, err := consumeResponsesEvent(raw, &result, responseCalls, &responseOrder)
			if err != nil {
				return StreamResult{}, err
			}
			if completed {
				responseCompleted = true
				completedOutputCalls += outputCalls
			}
		default:
			return StreamResult{}, protocolError("unsupported_protocol", "unsupported AI stream protocol", nil)
		}
	}
	if err := scanner.Err(); err != nil {
		return StreamResult{}, protocolError("stream_read_error", "failed to read AI stream", err)
	}

	switch mode {
	case ProtocolChatCompletions:
		if result.TerminalFrames == 0 {
			return StreamResult{}, protocolError("incomplete_stream", "Chat Completions stream ended without [DONE]", nil)
		}
		indexes := make([]int, 0, len(chatCalls))
		for index := range chatCalls {
			indexes = append(indexes, index)
		}
		sort.Ints(indexes)
		for _, index := range indexes {
			call, err := finalizeToolCall(chatCalls[index])
			if err != nil {
				return StreamResult{}, err
			}
			result.ToolCalls = append(result.ToolCalls, call)
		}
	case ProtocolResponses:
		if !responseCompleted {
			return StreamResult{}, protocolError("incomplete_stream", "Responses stream ended without response.completed", nil)
		}
		completeObserved := false
		for _, key := range responseOrder {
			builder := responseCalls[key]
			if builder.name == "" {
				continue
			}
			call, err := finalizeToolCall(builder)
			if err != nil {
				return StreamResult{}, err
			}
			result.ToolCalls = append(result.ToolCalls, call)
			completeObserved = completeObserved || builder.complete
		}
		result.RepairedOutput = completedOutputCalls == 0 && completeObserved && len(result.ToolCalls) > 0
	}

	return result, nil
}

func consumeChatChunk(raw map[string]any, result *StreamResult, calls map[int]*toolCallBuilder) error {
	choices, _ := raw["choices"].([]any)
	for _, choiceValue := range choices {
		choice, _ := choiceValue.(map[string]any)
		delta, _ := choice["delta"].(map[string]any)
		if content, ok := delta["content"].(string); ok {
			result.Text += content
		}
		toolCalls, _ := delta["tool_calls"].([]any)
		for _, toolValue := range toolCalls {
			tool, _ := toolValue.(map[string]any)
			index := int(numberValue(tool["index"]))
			builder := calls[index]
			if builder == nil {
				builder = &toolCallBuilder{index: index}
				calls[index] = builder
			}
			if id, ok := tool["id"].(string); ok && id != "" {
				builder.id = id
			}
			function, _ := tool["function"].(map[string]any)
			if name, ok := function["name"].(string); ok && name != "" {
				builder.name = name
			}
			if arguments, ok := function["arguments"].(string); ok {
				builder.arguments.WriteString(arguments)
			}
		}
	}
	return nil
}

func consumeResponsesEvent(raw map[string]any, result *StreamResult, calls map[string]*toolCallBuilder, order *[]string) (bool, int, error) {
	eventType, _ := raw["type"].(string)
	switch eventType {
	case "response.output_text.delta":
		if delta, ok := raw["delta"].(string); ok {
			result.Text += delta
		}
	case "response.output_item.added", "response.output_item.done":
		item, _ := raw["item"].(map[string]any)
		if item["type"] == "function_call" {
			mergeResponseCall(item, calls, order, eventType == "response.output_item.done")
		}
	case "response.function_call_arguments.delta":
		key, _ := raw["item_id"].(string)
		builder := responseBuilder(key, calls, order)
		if delta, ok := raw["delta"].(string); ok {
			builder.arguments.WriteString(delta)
		}
	case "response.function_call_arguments.done":
		key, _ := raw["item_id"].(string)
		builder := responseBuilder(key, calls, order)
		if arguments, ok := raw["arguments"].(string); ok {
			builder.arguments.Reset()
			builder.arguments.WriteString(arguments)
		}
		builder.complete = true
	case "response.completed":
		response, _ := raw["response"].(map[string]any)
		output, _ := response["output"].([]any)
		count := 0
		for _, itemValue := range output {
			item, _ := itemValue.(map[string]any)
			if item["type"] != "function_call" {
				continue
			}
			mergeResponseCall(item, calls, order, true)
			count++
		}
		return true, count, nil
	}
	return false, 0, nil
}

func mergeResponseCall(item map[string]any, calls map[string]*toolCallBuilder, order *[]string, complete bool) {
	key, _ := item["id"].(string)
	if key == "" {
		key, _ = item["call_id"].(string)
	}
	builder := responseBuilder(key, calls, order)
	if id, ok := item["call_id"].(string); ok && id != "" {
		builder.id = id
	}
	if name, ok := item["name"].(string); ok && name != "" {
		builder.name = name
	}
	if arguments, ok := item["arguments"].(string); ok && arguments != "" {
		builder.arguments.Reset()
		builder.arguments.WriteString(arguments)
	}
	if complete {
		builder.complete = true
	}
}

func responseBuilder(key string, calls map[string]*toolCallBuilder, order *[]string) *toolCallBuilder {
	if key == "" {
		key = fmt.Sprintf("anonymous-%d", len(*order))
	}
	if builder := calls[key]; builder != nil {
		return builder
	}
	builder := &toolCallBuilder{index: len(*order)}
	calls[key] = builder
	*order = append(*order, key)
	return builder
}

func finalizeToolCall(builder *toolCallBuilder) (ToolCall, error) {
	arguments := strings.TrimSpace(builder.arguments.String())
	if arguments == "" {
		arguments = "{}"
	}
	if !json.Valid([]byte(arguments)) {
		return ToolCall{}, protocolError("malformed_tool_arguments", "tool call arguments are not valid JSON", nil)
	}
	return ToolCall{ID: builder.id, Name: builder.name, Arguments: json.RawMessage(arguments)}, nil
}

func numberValue(value any) float64 {
	switch number := value.(type) {
	case float64:
		return number
	case json.Number:
		parsed, _ := number.Float64()
		return parsed
	default:
		return 0
	}
}

type ToolDefinition struct {
	Name string
}

type ToolRoundRequest struct {
	Round      int
	Tools      []ToolDefinition
	ToolChoice string
}

type ToolRoundTransport interface {
	CallToolRound(context.Context, ToolRoundRequest) (io.ReadCloser, ProtocolMode, error)
}

// RequestRequiredTool retries only the safe no-tool outcome. Protocol and
// transport failures remain explicit terminal errors for the current cycle.
func RequestRequiredTool(ctx context.Context, transport ToolRoundTransport, tools []ToolDefinition, maxRounds int, roundTimeout time.Duration) (ToolCall, error) {
	if maxRounds <= 0 {
		maxRounds = 3
	}
	hold := ToolDefinition{Name: "hold"}
	for _, tool := range tools {
		if tool.Name == "hold" {
			hold = tool
			break
		}
	}

	for round := 1; round <= maxRounds; round++ {
		if err := ctx.Err(); err != nil {
			return ToolCall{}, protocolError("cycle_timeout", "AI cycle deadline exceeded", err)
		}
		requestTools := append([]ToolDefinition(nil), tools...)
		if round == maxRounds {
			requestTools = []ToolDefinition{hold}
		}
		request := ToolRoundRequest{Round: round, Tools: requestTools, ToolChoice: "required"}

		roundCtx := ctx
		cancel := func() {}
		if roundTimeout > 0 {
			roundCtx, cancel = context.WithTimeout(ctx, roundTimeout)
		}
		body, mode, err := transport.CallToolRound(roundCtx, request)
		if err != nil {
			contextErr := roundCtx.Err()
			cancel()
			if contextErr != nil {
				return ToolCall{}, protocolError("round_timeout", "AI tool round deadline exceeded", contextErr)
			}
			return ToolCall{}, protocolError("upstream_transport", "AI tool round transport failed", err)
		}
		result, normalizeErr := NormalizeSSE(mode, body)
		closeErr := body.Close()
		cancel()
		if normalizeErr != nil {
			return ToolCall{}, normalizeErr
		}
		if closeErr != nil {
			return ToolCall{}, protocolError("stream_close_error", "failed to close AI stream", closeErr)
		}
		if len(result.ToolCalls) == 0 {
			continue
		}
		if len(result.ToolCalls) != 1 {
			return ToolCall{}, protocolError("multiple_tool_calls", "AI returned more than one trading tool call", nil)
		}
		call := result.ToolCalls[0]
		if !toolAllowed(call.Name, requestTools) {
			return ToolCall{}, protocolError("unallowed_tool", "AI returned a tool not allowed in this round", nil)
		}
		return call, nil
	}

	return ToolCall{}, protocolError("tool_call_missing", "model did not produce a function call after configured rounds", nil)
}

func toolAllowed(name string, tools []ToolDefinition) bool {
	for _, tool := range tools {
		if name == tool.Name {
			return true
		}
	}
	return false
}
