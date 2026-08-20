package parity

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestAIProtocolChatIgnoresCommentsAndDuplicateDone(t *testing.T) {
	t.Parallel()

	stream := strings.Join([]string{
		": OPENROUTER PROCESSING",
		`data: {"choices":[{"delta":{"content":"market "}}]}`,
		`data: {"choices":[{"delta":{"content":"stable"}}]}`,
		"data: [DONE]",
		"data: [DONE]",
		"",
	}, "\n")

	result, err := NormalizeSSE(ProtocolChatCompletions, strings.NewReader(stream))
	if err != nil {
		t.Fatalf("normalize stream: %v", err)
	}
	if result.Text != "market stable" || result.TerminalFrames != 1 {
		t.Fatalf("result = %+v", result)
	}
}

func TestAIProtocolRepairsResponsesOutputFromCompletedArguments(t *testing.T) {
	t.Parallel()

	stream := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"id":"item-1","call_id":"call-1","type":"function_call","name":"hold","arguments":""}}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"item-1","delta":"{\"symbol\":\"ALL\","}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"item-1","delta":"\"confidence\":65}"}`,
		`data: {"type":"response.function_call_arguments.done","item_id":"item-1","arguments":"{\"symbol\":\"ALL\",\"confidence\":65}"}`,
		`data: {"type":"response.completed","response":{"output":[]}}`,
		`data: [DONE]`,
	}, "\n")

	result, err := NormalizeSSE(ProtocolResponses, strings.NewReader(stream))
	if err != nil {
		t.Fatalf("normalize stream: %v", err)
	}
	if !result.RepairedOutput || len(result.ToolCalls) != 1 {
		t.Fatalf("result = %+v", result)
	}
	call := result.ToolCalls[0]
	if call.ID != "call-1" || call.Name != "hold" || string(call.Arguments) != `{"symbol":"ALL","confidence":65}` {
		t.Fatalf("tool call = %+v", call)
	}
}

func TestAIProtocolRejectsIncompleteResponsesStream(t *testing.T) {
	t.Parallel()

	stream := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"id":"item-1","call_id":"call-1","type":"function_call","name":"hold","arguments":""}}`,
		`data: {"type":"response.function_call_arguments.done","item_id":"item-1","arguments":"{}"}`,
		`data: [DONE]`,
	}, "\n")

	_, err := NormalizeSSE(ProtocolResponses, strings.NewReader(stream))
	assertProtocolErrorCode(t, err, "incomplete_stream")
}

func TestAIProtocolRejectsMalformedJSONFrame(t *testing.T) {
	t.Parallel()

	_, err := NormalizeSSE(ProtocolChatCompletions, strings.NewReader("data: {broken}\ndata: [DONE]\n"))
	assertProtocolErrorCode(t, err, "malformed_sse_json")
}

func TestAIProtocolRejectsChatWithoutDone(t *testing.T) {
	t.Parallel()

	_, err := NormalizeSSE(
		ProtocolChatCompletions,
		strings.NewReader(`data: {"choices":[{"delta":{"content":"partial"}}]}`+"\n"),
	)
	assertProtocolErrorCode(t, err, "incomplete_stream")
}

func TestAIProtocolRejectsMalformedToolArguments(t *testing.T) {
	t.Parallel()

	stream := chatToolStream("call-1", "hold", `{broken}`)
	_, err := NormalizeSSE(ProtocolChatCompletions, strings.NewReader(stream))
	assertProtocolErrorCode(t, err, "malformed_tool_arguments")
}

func TestAIProtocolMapsUpstreamHTTPFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status int
		code   string
	}{
		{status: 404, code: "upstream_route_unavailable"},
		{status: 429, code: "upstream_rate_limited"},
		{status: 500, code: "upstream_server_error"},
	}
	for _, tt := range tests {
		err := ProtocolErrorFromHTTP(tt.status, []byte(`{"error":"redacted"}`))
		if err.Code != tt.code || err.StatusCode != tt.status {
			t.Fatalf("status %d mapped to %+v", tt.status, err)
		}
	}
}

func TestAIProtocolRequiredToolFallsBackToHoldOnFinalRound(t *testing.T) {
	t.Parallel()

	transport := &scriptedToolTransport{streams: []string{
		chatTextStream("analysis only"),
		chatTextStream("still no tool"),
		chatToolStream("call-hold", "hold", `{"symbol":"ALL","confidence":50}`),
	}}
	tools := []ToolDefinition{{Name: "open_long"}, {Name: "open_short"}, {Name: "hold"}}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	call, err := RequestRequiredTool(ctx, transport, tools, 3, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("request required tool: %v", err)
	}
	if call.Name != "hold" || len(transport.requests) != 3 {
		t.Fatalf("call=%+v requests=%+v", call, transport.requests)
	}
	last := transport.requests[2]
	if last.ToolChoice != "required" || len(last.Tools) != 1 || last.Tools[0].Name != "hold" {
		t.Fatalf("final request = %+v", last)
	}
}

func TestAIProtocolRequiredToolStopsAfterConfiguredRounds(t *testing.T) {
	t.Parallel()

	transport := &scriptedToolTransport{streams: []string{chatTextStream("one"), chatTextStream("two")}}
	_, err := RequestRequiredTool(
		context.Background(),
		transport,
		[]ToolDefinition{{Name: "hold"}},
		2,
		100*time.Millisecond,
	)
	assertProtocolErrorCode(t, err, "tool_call_missing")
	if len(transport.requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(transport.requests))
	}
}

func TestAIProtocolRequiredToolRejectsUnallowedTool(t *testing.T) {
	t.Parallel()

	transport := &scriptedToolTransport{streams: []string{
		chatToolStream("call-1", "open_short", `{"symbol":"BTCUSDT"}`),
	}}
	_, err := RequestRequiredTool(
		context.Background(),
		transport,
		[]ToolDefinition{{Name: "hold"}},
		1,
		100*time.Millisecond,
	)
	assertProtocolErrorCode(t, err, "unallowed_tool")
}

func TestAIProtocolKeepsImmediateTransportFailureDistinctFromTimeout(t *testing.T) {
	t.Parallel()

	_, err := RequestRequiredTool(
		context.Background(),
		failingToolTransport{},
		[]ToolDefinition{{Name: "hold"}},
		1,
		100*time.Millisecond,
	)
	assertProtocolErrorCode(t, err, "upstream_transport")
}

func assertProtocolErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected protocol error %q", code)
	}
	var protocolErr *ProtocolError
	if !errors.As(err, &protocolErr) {
		t.Fatalf("error type = %T, want *ProtocolError", err)
	}
	if protocolErr.Code != code {
		t.Fatalf("error code = %q, want %q", protocolErr.Code, code)
	}
}

func chatTextStream(text string) string {
	return `data: {"choices":[{"delta":{"content":"` + text + `"}}]}` + "\ndata: [DONE]\n"
}

func chatToolStream(id, name, arguments string) string {
	return `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"` + id + `","type":"function","function":{"name":"` + name + `","arguments":` + quoteJSON(arguments) + `}}]}}]}` + "\ndata: [DONE]\n"
}

func quoteJSON(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

type scriptedToolTransport struct {
	streams  []string
	requests []ToolRoundRequest
}

type failingToolTransport struct{}

func (failingToolTransport) CallToolRound(context.Context, ToolRoundRequest) (io.ReadCloser, ProtocolMode, error) {
	return nil, "", errors.New("connection reset")
}

func (s *scriptedToolTransport) CallToolRound(_ context.Context, request ToolRoundRequest) (io.ReadCloser, ProtocolMode, error) {
	s.requests = append(s.requests, request)
	if len(s.streams) == 0 {
		return nil, "", errors.New("unexpected request")
	}
	stream := s.streams[0]
	s.streams = s.streams[1:]
	return io.NopCloser(strings.NewReader(stream)), ProtocolChatCompletions, nil
}
