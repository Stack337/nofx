package execution

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"

	"nofx/agent"
	"nofx/risk"
)

type Router struct {
	exchange  Exchange
	mu        sync.Mutex
	completed map[string]OrderResult
}

func NewRouter(exchange Exchange) *Router {
	return &Router{exchange: exchange, completed: make(map[string]OrderResult)}
}

func (r *Router) Execute(ctx context.Context, mode agent.Mode, authorized risk.AuthorizedDecision, cycleID string) (OrderResult, error) {
	if err := ctx.Err(); err != nil {
		return OrderResult{}, err
	}
	decision := authorized.Decision
	if decision.Action == agent.DecisionHold {
		return OrderResult{Status: OrderStatusPaper}, nil
	}
	if mode == agent.ModeShadow {
		return OrderResult{Status: OrderStatusShadow}, nil
	}
	if mode == agent.ModePaper {
		return OrderResult{OrderID: "paper-" + cycleID, Status: OrderStatusPaper}, nil
	}
	if mode != agent.ModeLive || !authorized.LiveConfirmed {
		return OrderResult{}, ErrLiveGate
	}
	key := fmt.Sprintf("%s:%s:%s:%s", cycleID, decision.Action, decision.Symbol, decision.Side)
	r.mu.Lock()
	if previous, ok := r.completed[key]; ok {
		r.mu.Unlock()
		return previous, nil
	}
	r.mu.Unlock()

	modeValue, err := r.exchange.PositionMode(ctx, decision.Symbol)
	if err != nil {
		return OrderResult{}, err
	}
	if modeValue != PositionModeOneWay {
		return OrderResult{}, &ExchangeError{Code: "position_mode_mismatch", Message: "live execution requires one-way position mode"}
	}
	instrument, err := r.exchange.Instrument(ctx, decision.Symbol)
	if err != nil {
		return OrderResult{}, err
	}
	quantity := roundDown(decision.Quantity, instrument.QtyStep)
	if quantity <= 0 {
		return OrderResult{}, fmt.Errorf("quantity rounds to zero")
	}
	reduceOnly := decision.Action == agent.DecisionClose
	positionIdx := positionIndex(modeValue, decision.Side)
	orderSide := normalizedSide(decision.Side)
	if reduceOnly {
		orderSide = strings.ToLower(strings.TrimSpace(decision.Side))
	}
	result, err := r.exchange.Place(ctx, OrderRequest{
		Symbol: decision.Symbol, Side: orderSide, Quantity: quantity,
		Leverage: decision.Leverage, ReduceOnly: reduceOnly, PositionIdx: positionIdx, ClientID: key,
	})
	if err != nil {
		return OrderResult{}, err
	}
	if result.Status == OrderStatusUnknown {
		state, statusErr := r.exchange.GetOrder(ctx, decision.Symbol, result.OrderID)
		if statusErr != nil || state.Status == OrderStatusUnknown {
			return OrderResult{OrderID: result.OrderID, Status: OrderStatusUnknown}, ErrNeedsReconciliation
		}
		result = state
	}
	if decision.Action == agent.DecisionOpen && (decision.StopLoss != nil || decision.TakeProfit != nil) {
		if err := r.exchange.SetProtection(ctx, ProtectionRequest{
			Symbol: decision.Symbol, Side: decision.Side, Quantity: quantity,
			StopLoss: decision.StopLoss, TakeProfit: decision.TakeProfit, PositionIdx: positionIdx,
		}); err != nil {
			return result, fmt.Errorf("protection placement failed: %w", err)
		}
	}
	r.mu.Lock()
	r.completed[key] = result
	r.mu.Unlock()
	return result, nil
}

func positionIndex(mode PositionMode, side string) int {
	if mode != PositionModeHedge {
		return 0
	}
	if strings.EqualFold(side, "sell") || strings.EqualFold(side, "short") {
		return 2
	}
	return 1
}

func normalizedSide(side string) string {
	if strings.EqualFold(side, "sell") || strings.EqualFold(side, "short") {
		return "Sell"
	}
	return "Buy"
}

func roundDown(value, step float64) float64 {
	if step <= 0 {
		return value
	}
	return math.Floor(value/step+1e-9) * step
}
