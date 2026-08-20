package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"nofx/agent"
)

const (
	DecisionErrorEmptyResponse       = "empty_response"
	DecisionErrorInvalidJSON         = "invalid_json"
	DecisionErrorMissingAction       = "missing_action"
	DecisionErrorUnsupportedFunction = "unsupported_function"
	DecisionErrorInvalidSymbol       = "invalid_symbol"
	DecisionErrorInvalidParameters   = "invalid_parameters"
	DecisionErrorTimeout             = "provider_timeout"
)

type DecisionError struct {
	Code    string
	Message string
	Cause   error
}

func (e *DecisionError) Error() string { return e.Message }
func (e *DecisionError) Unwrap() error { return e.Cause }

type PositionView struct {
	Symbol, Side                                   string
	Quantity, EntryPrice, MarkPrice, UnrealizedPnL float64
}

type RiskView struct {
	Equity, AvailableBalance, DailyPnL, OpenNotional float64
	OpenPositions                                    int
}

type CandidateView struct {
	Symbol     string
	Score      float64
	LongScore  float64
	ShortScore float64
	Confidence float64
	Regime     string
	Reasons    []string
}

type DecisionRequest struct {
	CycleID        string
	AgentID        string
	Candidates     []CandidateView
	Positions      []PositionView
	Risk           RiskView
	AllowedActions []agent.DecisionAction
}

type DecisionResponse struct {
	Action      agent.DecisionAction `json:"action"`
	Symbol      string               `json:"symbol"`
	Side        string               `json:"side"`
	Quantity    float64              `json:"quantity"`
	Leverage    int                  `json:"leverage"`
	StopLoss    *float64             `json:"stop_loss"`
	TakeProfit  *float64             `json:"take_profit"`
	Confidence  float64              `json:"confidence"`
	Rationale   string               `json:"rationale"`
	GeneratedAt time.Time            `json:"-"`
}

type ProviderMeta struct {
	Provider, Model, RequestID string
	Latency                    time.Duration
}

type AgentProvider interface {
	Decide(context.Context, DecisionRequest) (DecisionResponse, ProviderMeta, error)
}

func DecideWithTimeout(parent context.Context, timeout time.Duration, provider AgentProvider, request DecisionRequest) (DecisionResponse, ProviderMeta, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	decision, meta, err := provider.Decide(ctx, request)
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return DecisionResponse{}, meta, decisionError(DecisionErrorTimeout, "AI provider request timed out", context.DeadlineExceeded)
	}
	return decision, meta, err
}

func ParseAgentDecision(body []byte, allowedSymbols []string) (DecisionResponse, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return DecisionResponse{}, decisionError(DecisionErrorEmptyResponse, "AI response is empty", nil)
	}
	payload, err := extractDecisionPayload([]byte(trimmed))
	if err != nil {
		return DecisionResponse{}, err
	}
	var decision DecisionResponse
	if err := json.Unmarshal(payload, &decision); err != nil {
		return DecisionResponse{}, decisionError(DecisionErrorInvalidJSON, "AI decision arguments are not valid JSON", err)
	}
	decision.Symbol = strings.ToUpper(strings.TrimSpace(decision.Symbol))
	decision.Side = strings.ToLower(strings.TrimSpace(decision.Side))
	decision.Rationale = strings.TrimSpace(decision.Rationale)
	if decision.Action == "" {
		return DecisionResponse{}, decisionError(DecisionErrorMissingAction, "AI decision action is required", nil)
	}
	if decision.Action != agent.DecisionOpen && decision.Action != agent.DecisionClose && decision.Action != agent.DecisionHold {
		return DecisionResponse{}, decisionError(DecisionErrorInvalidParameters, fmt.Sprintf("unsupported action %q", decision.Action), nil)
	}
	if decision.Action == agent.DecisionHold {
		return decision, nil
	}
	if !containsSymbol(allowedSymbols, decision.Symbol) {
		return DecisionResponse{}, decisionError(DecisionErrorInvalidSymbol, fmt.Sprintf("symbol %q is not an allowed candidate", decision.Symbol), nil)
	}
	if decision.Action == agent.DecisionOpen {
		if (decision.Side != "buy" && decision.Side != "sell" && decision.Side != "long" && decision.Side != "short") || decision.Quantity <= 0 || decision.Leverage <= 0 {
			return DecisionResponse{}, decisionError(DecisionErrorInvalidParameters, "open decision requires side, positive quantity, and leverage", nil)
		}
	}
	if math.IsNaN(decision.Confidence) || math.IsInf(decision.Confidence, 0) || decision.Confidence < 0 || decision.Confidence > 100 {
		return DecisionResponse{}, decisionError(DecisionErrorInvalidParameters, "confidence must be between 0 and 100", nil)
	}
	return decision, nil
}

func extractDecisionPayload(body []byte) ([]byte, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, decisionError(DecisionErrorInvalidJSON, "AI response is not valid JSON", err)
	}
	if _, direct := root["action"]; direct {
		return body, nil
	}
	if choices, ok := root["choices"]; ok {
		var items []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		}
		if json.Unmarshal(choices, &items) == nil && len(items) > 0 {
			if len(items[0].Message.ToolCalls) > 0 {
				call := items[0].Message.ToolCalls[0].Function
				if call.Name != "trade_decision" && call.Name != "plan" {
					return nil, decisionError(DecisionErrorUnsupportedFunction, fmt.Sprintf("unsupported function %q", call.Name), nil)
				}
				return []byte(call.Arguments), nil
			}
			if strings.TrimSpace(items[0].Message.Content) != "" {
				return []byte(items[0].Message.Content), nil
			}
		}
	}
	if output, ok := root["output"]; ok {
		var items []struct {
			Type      string `json:"type"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
			Content   []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if json.Unmarshal(output, &items) == nil {
			for _, item := range items {
				if item.Type == "function_call" {
					if item.Name != "trade_decision" && item.Name != "plan" {
						return nil, decisionError(DecisionErrorUnsupportedFunction, fmt.Sprintf("unsupported function %q", item.Name), nil)
					}
					return []byte(item.Arguments), nil
				}
				for _, content := range item.Content {
					if content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
						return []byte(content.Text), nil
					}
				}
			}
		}
	}
	return nil, decisionError(DecisionErrorMissingAction, "AI response contains no decision or supported function call", nil)
}

func containsSymbol(symbols []string, symbol string) bool {
	for _, candidate := range symbols {
		if strings.EqualFold(strings.TrimSpace(candidate), symbol) {
			return true
		}
	}
	return false
}

func decisionError(code, message string, cause error) *DecisionError {
	return &DecisionError{Code: code, Message: message, Cause: cause}
}

func ClassifyHTTPStatus(status int) *DecisionError {
	switch status {
	case 429:
		return decisionError("provider_http_429", "AI provider rate limited the request", nil)
	case 404:
		return decisionError("provider_http_404", "AI provider endpoint was not found", nil)
	case 500:
		return decisionError("provider_http_500", "AI provider returned an internal error", nil)
	default:
		return decisionError(fmt.Sprintf("provider_http_%d", status), "AI provider returned an unexpected HTTP status", nil)
	}
}
