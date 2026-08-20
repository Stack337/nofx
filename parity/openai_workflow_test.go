package parity

import (
	"context"
	"testing"
	"time"

	"nofx/mcp"
)

func TestOpenAIWorkflowRetriesMissingToolAndFallsBackToHold(t *testing.T) {
	t.Parallel()

	client := &fakeParityAIClient{responses: []*mcp.LLMResponse{
		{Content: "no tool"},
		{Content: "still no tool"},
		{ToolCalls: []mcp.ToolCall{{ID: "call-1", Type: "function", Function: mcp.ToolCallFunction{
			Name: "hold", Arguments: `{"symbol":"ALL","confidence":50}`,
		}}}},
	}}
	workflow := NewOpenAIWorkflow(client, "deepseek-v4-flash", 3)

	decision, err := workflow.Decide(context.Background(), "analysis", &TradingContextSnapshot{})
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if decision.Action != ActionHold || decision.Symbol != "ALL" {
		t.Fatalf("decision = %+v", decision)
	}
	if len(client.requests) != 3 {
		t.Fatalf("requests = %d, want 3", len(client.requests))
	}
	last := client.requests[2]
	if last.ToolChoice != "required" || len(last.Tools) != 1 || last.Tools[0].Function.Name != "hold" {
		t.Fatalf("final request = %+v", last)
	}
}

func TestOpenAIWorkflowAnalyzeUsesSnapshotWithoutSecrets(t *testing.T) {
	t.Parallel()

	client := &fakeParityAIClient{streamText: "market analysis"}
	workflow := NewOpenAIWorkflow(client, "deepseek-v4-flash", 3)
	snapshot := &TradingContextSnapshot{Account: AccountSnapshot{TotalEquity: 100}}

	analysis, err := workflow.Analyze(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if analysis != "market analysis" || len(client.requests) != 1 {
		t.Fatalf("analysis=%q requests=%d", analysis, len(client.requests))
	}
}

func TestOpenAIWorkflowEnrichesOpenDecisionFromMarketSnapshot(t *testing.T) {
	t.Parallel()

	client := &fakeParityAIClient{responses: []*mcp.LLMResponse{{ToolCalls: []mcp.ToolCall{{
		ID: "call-open", Type: "function", Function: mcp.ToolCallFunction{Name: "open_long", Arguments: `{"symbol":"BTCUSDT","leverage":3,"position_size_usd":60,"stop_loss":59000,"take_profit":66000}`},
	}}}}}
	workflow := NewOpenAIWorkflow(client, "", 3)
	now := time.Now().UTC()
	snapshot := &TradingContextSnapshot{Markets: map[string]MarketSnapshot{"BTCUSDT": {
		CurrentPrice: 61000, PriceTimestamp: now, PriceDivergencePct: 0.1,
	}}}

	decision, err := workflow.Decide(context.Background(), "analysis", snapshot)
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if decision.MarketPrice != 61000 || !decision.PriceTimestamp.Equal(now) {
		t.Fatalf("decision = %+v", decision)
	}
}

type fakeParityAIClient struct {
	responses  []*mcp.LLMResponse
	streamText string
	requests   []*mcp.Request
}

func (f *fakeParityAIClient) CallWithRequestStream(req *mcp.Request, _ func(string)) (string, error) {
	f.requests = append(f.requests, req)
	return f.streamText, nil
}

func (f *fakeParityAIClient) CallWithRequestFull(req *mcp.Request) (*mcp.LLMResponse, error) {
	f.requests = append(f.requests, req)
	response := f.responses[0]
	f.responses = f.responses[1:]
	return response, nil
}
