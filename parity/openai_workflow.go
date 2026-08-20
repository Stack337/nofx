package parity

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"nofx/mcp"
)

type parityAIClient interface {
	CallWithRequestStream(*mcp.Request, func(string)) (string, error)
	CallWithRequestFull(*mcp.Request) (*mcp.LLMResponse, error)
}

type OpenAIWorkflow struct {
	client    parityAIClient
	model     string
	maxRounds int
}

func NewOpenAIWorkflow(client parityAIClient, model string, maxRounds int) *OpenAIWorkflow {
	if maxRounds <= 0 {
		maxRounds = 3
	}
	return &OpenAIWorkflow{client: client, model: strings.TrimSpace(model), maxRounds: maxRounds}
}

func (w *OpenAIWorkflow) Analyze(ctx context.Context, snapshot *TradingContextSnapshot) (AnalysisText, error) {
	prompt, err := json.Marshal(snapshot)
	if err != nil {
		return "", fmt.Errorf("marshal trading snapshot: %w", err)
	}
	maxTokens := 4096
	request := &mcp.Request{
		Ctx: ctx, Model: w.model, Stream: true, MaxTokens: &maxTokens,
		Messages: []mcp.Message{
			mcp.NewSystemMessage("Analyze the supplied market snapshot. Return text analysis only; do not emit trading actions."),
			mcp.NewUserMessage(string(prompt)),
		},
	}
	text, err := w.client.CallWithRequestStream(request, nil)
	if err != nil {
		return "", fmt.Errorf("analysis round: %w", err)
	}
	if strings.TrimSpace(text) == "" {
		return "", protocolError("empty_analysis", "AI analysis round returned no text", nil)
	}
	return AnalysisText(text), nil
}

func (w *OpenAIWorkflow) Decide(ctx context.Context, analysis AnalysisText, snapshot *TradingContextSnapshot) (DecisionInput, error) {
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return DecisionInput{}, fmt.Errorf("marshal trading snapshot: %w", err)
	}
	tools := tradingTools()
	for round := 1; round <= w.maxRounds; round++ {
		requestTools := tools
		if round == w.maxRounds {
			requestTools = []mcp.Tool{holdTool()}
		}
		request := &mcp.Request{
			Ctx: ctx, Model: w.model, ToolChoice: "required", Tools: requestTools,
			Messages: []mcp.Message{
				mcp.NewSystemMessage("Submit exactly one trading decision using an available function. Never return the decision as plain text."),
				mcp.NewUserMessage("Market analysis:\n" + string(analysis) + "\n\nSnapshot:\n" + string(snapshotJSON)),
			},
		}
		response, callErr := w.client.CallWithRequestFull(request)
		if callErr != nil {
			return DecisionInput{}, fmt.Errorf("tool round: %w", callErr)
		}
		if response == nil || len(response.ToolCalls) == 0 {
			continue
		}
		if len(response.ToolCalls) != 1 {
			return DecisionInput{}, protocolError("multiple_tool_calls", "AI returned more than one trading tool call", nil)
		}
		call := response.ToolCalls[0]
		if !mcpToolAllowed(call.Function.Name, requestTools) {
			return DecisionInput{}, protocolError("unallowed_tool", "AI returned a tool not allowed in this round", nil)
		}
		decision, parseErr := decisionFromTool(call.Function.Name, call.Function.Arguments)
		if parseErr != nil {
			return DecisionInput{}, parseErr
		}
		if marketSnapshot, ok := snapshot.Markets[decision.Symbol]; ok {
			decision.MarketPrice = marketSnapshot.CurrentPrice
			decision.PriceTimestamp = marketSnapshot.PriceTimestamp
			decision.PriceDivergencePct = marketSnapshot.PriceDivergencePct
		}
		return decision, nil
	}
	return DecisionInput{}, protocolError("tool_call_missing", "model did not produce a function call after configured rounds", nil)
}

func tradingTools() []mcp.Tool {
	return []mcp.Tool{decisionTool("open_long"), decisionTool("open_short"), decisionTool("close_long"), decisionTool("close_short"), holdTool()}
}

func decisionTool(name string) mcp.Tool {
	return mcp.Tool{Type: "function", Function: mcp.FunctionDef{
		Name: name, Description: "Submit one risk-bounded trading decision",
		Parameters: map[string]any{"type": "object", "required": []string{"symbol"}, "properties": map[string]any{
			"symbol": map[string]any{"type": "string"}, "leverage": map[string]any{"type": "integer"},
			"position_size_usd": map[string]any{"type": "number"}, "quantity": map[string]any{"type": "number"},
			"stop_loss": map[string]any{"type": "number"}, "take_profit": map[string]any{"type": "number"},
		}},
	}}
}

func holdTool() mcp.Tool {
	return mcp.Tool{Type: "function", Function: mcp.FunctionDef{
		Name: "hold", Description: "Safely take no trading action",
		Parameters: map[string]any{"type": "object", "required": []string{"symbol"}, "properties": map[string]any{
			"symbol": map[string]any{"type": "string"}, "confidence": map[string]any{"type": "number"},
		}},
	}}
}

func mcpToolAllowed(name string, tools []mcp.Tool) bool {
	for _, tool := range tools {
		if tool.Function.Name == name {
			return true
		}
	}
	return false
}

func decisionFromTool(name, arguments string) (DecisionInput, error) {
	var payload struct {
		Symbol          string  `json:"symbol"`
		Leverage        int     `json:"leverage"`
		PositionSizeUSD float64 `json:"position_size_usd"`
		Quantity        float64 `json:"quantity"`
		StopLoss        float64 `json:"stop_loss"`
		TakeProfit      float64 `json:"take_profit"`
	}
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return DecisionInput{}, protocolError("malformed_tool_arguments", "tool call arguments are not valid JSON", err)
	}
	action := Action(name)
	switch action {
	case ActionOpenLong, ActionOpenShort, ActionCloseLong, ActionCloseShort, ActionHold:
	default:
		return DecisionInput{}, protocolError("unallowed_tool", "AI returned an unsupported trading tool", nil)
	}
	return DecisionInput{
		Symbol: strings.ToUpper(strings.TrimSpace(payload.Symbol)), Action: action, Leverage: payload.Leverage,
		PositionSizeUSD: payload.PositionSizeUSD, Quantity: payload.Quantity,
		StopLoss: payload.StopLoss, TakeProfit: payload.TakeProfit,
	}, nil
}
