package provider

import (
	"errors"
	"testing"
)

func TestFailureInjectionRejectsTruncatedJSONStream(t *testing.T) {
	_, err := ParseAgentDecision([]byte(`{"action":"open","symbol":"BTCUSDT"`), []string{"BTCUSDT"})
	var decisionErr *DecisionError
	if !errors.As(err, &decisionErr) || decisionErr.Code != DecisionErrorInvalidJSON {
		t.Fatalf("error = %v, want %s", err, DecisionErrorInvalidJSON)
	}
}

func TestFailureInjectionRejectsUnsupportedFunction(t *testing.T) {
	body := []byte(`{"choices":[{"message":{"tool_calls":[{"function":{"name":"withdraw","arguments":"{}"}}]}}]}`)
	_, err := ParseAgentDecision(body, []string{"BTCUSDT"})
	var decisionErr *DecisionError
	if !errors.As(err, &decisionErr) || decisionErr.Code != DecisionErrorUnsupportedFunction {
		t.Fatalf("error = %v, want %s", err, DecisionErrorUnsupportedFunction)
	}
}

func TestFailureInjectionClassifiesProviderHTTPStatuses(t *testing.T) {
	cases := map[int]string{429: "provider_http_429", 404: "provider_http_404", 500: "provider_http_500"}
	for status, code := range cases {
		err := ClassifyHTTPStatus(status)
		var decisionErr *DecisionError
		if !errors.As(err, &decisionErr) || decisionErr.Code != code {
			t.Fatalf("status %d error = %v, want %s", status, err, code)
		}
	}
}
