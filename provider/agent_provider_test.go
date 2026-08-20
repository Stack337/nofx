package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"nofx/agent"
)

type blockingAgentProvider struct{}

func (blockingAgentProvider) Decide(ctx context.Context, _ DecisionRequest) (DecisionResponse, ProviderMeta, error) {
	<-ctx.Done()
	return DecisionResponse{}, ProviderMeta{}, ctx.Err()
}

func TestParseAgentDecisionReadsFunctionCallArguments(t *testing.T) {
	body := []byte(`{"choices":[{"message":{"tool_calls":[{"type":"function","function":{"name":"trade_decision","arguments":"{\"action\":\"open\",\"symbol\":\"BTCUSDT\",\"side\":\"buy\",\"quantity\":0.01,\"leverage\":3,\"confidence\":82}"}}]}}]}`)
	got, err := ParseAgentDecision(body, []string{"BTCUSDT"})
	if err != nil {
		t.Fatalf("ParseAgentDecision() error = %v", err)
	}
	if got.Action != agent.DecisionOpen || got.Symbol != "BTCUSDT" || got.Quantity != 0.01 {
		t.Fatalf("decision = %+v", got)
	}
}

func TestParseAgentDecisionRejectsEmptyResponse(t *testing.T) {
	_, err := ParseAgentDecision(nil, []string{"BTCUSDT"})
	var decisionErr *DecisionError
	if !errors.As(err, &decisionErr) || decisionErr.Code != DecisionErrorEmptyResponse {
		t.Fatalf("error = %v, want %s", err, DecisionErrorEmptyResponse)
	}
}

func TestParseAgentDecisionRejectsUnknownSymbol(t *testing.T) {
	_, err := ParseAgentDecision([]byte(`{"action":"open","symbol":"ETHUSDT","side":"buy","quantity":1,"leverage":2}`), []string{"BTCUSDT"})
	var decisionErr *DecisionError
	if !errors.As(err, &decisionErr) || decisionErr.Code != DecisionErrorInvalidSymbol {
		t.Fatalf("error = %v, want %s", err, DecisionErrorInvalidSymbol)
	}
}

func TestParseAgentDecisionRejectsMissingAction(t *testing.T) {
	_, err := ParseAgentDecision([]byte(`{"symbol":"BTCUSDT"}`), []string{"BTCUSDT"})
	var decisionErr *DecisionError
	if !errors.As(err, &decisionErr) || decisionErr.Code != DecisionErrorMissingAction {
		t.Fatalf("error = %v, want %s", err, DecisionErrorMissingAction)
	}
}

func TestDecideWithTimeoutClassifiesProviderDeadline(t *testing.T) {
	_, _, err := DecideWithTimeout(context.Background(), 10*time.Millisecond, blockingAgentProvider{}, DecisionRequest{})
	var decisionErr *DecisionError
	if !errors.As(err, &decisionErr) || decisionErr.Code != DecisionErrorTimeout {
		t.Fatalf("error = %v, want %s", err, DecisionErrorTimeout)
	}
}
