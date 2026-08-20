package parity

import (
	"context"
	"fmt"
	"sync"
)

type BybitOperation string

const (
	BybitCancelOrders      BybitOperation = "cancel_orders"
	BybitCancelConditional BybitOperation = "cancel_conditional"
	BybitSetLeverage       BybitOperation = "set_leverage"
	BybitPlaceOrder        BybitOperation = "place_order"
	BybitPlaceStopLoss     BybitOperation = "place_stop_loss"
	BybitPlaceTakeProfit   BybitOperation = "place_take_profit"
)

type BybitOrderRequest struct {
	Operation        BybitOperation
	Category         string
	Symbol           string
	Side             string
	OrderType        string
	Quantity         float64
	Leverage         int
	PositionIdx      int
	ReduceOnly       bool
	TriggerPrice     float64
	TriggerDirection int
	ReferencePrice   float64
}

type BybitResponse struct {
	Code    int
	Message string
}

type BybitExchangeFill struct {
	OrderID      string
	Quantity     float64
	AveragePrice float64
	Fee          float64
}

type ReconciledFill struct {
	OrderID      string
	Symbol       string
	Quantity     float64
	AveragePrice float64
	Fee          float64
}

type ExecutionResult struct {
	Simulated bool
	Requests  []BybitOrderRequest
	Fill      *ReconciledFill
}

type ExecutionPort interface {
	Execute(context.Context, DecisionInput) (ExecutionResult, error)
}

// BuildBybitExecutionPlan converts one validated decision into the exact V5
// one-way-mode request sequence without performing network I/O.
func BuildBybitExecutionPlan(decision DecisionInput) ([]BybitOrderRequest, error) {
	base := BybitOrderRequest{
		Category:    "linear",
		Symbol:      decision.Symbol,
		PositionIdx: 0,
	}

	switch decision.Action {
	case ActionOpenLong, ActionOpenShort:
		if decision.MarketPrice <= 0 || decision.PositionSizeUSD <= 0 {
			return nil, fmt.Errorf("open decision requires positive market price and position size")
		}
		quantity := decision.PositionSizeUSD / decision.MarketPrice
		entrySide := "Buy"
		exitSide := "Sell"
		stopDirection := 2
		takeDirection := 1
		if decision.Action == ActionOpenShort {
			entrySide = "Sell"
			exitSide = "Buy"
			stopDirection = 1
			takeDirection = 2
		}

		cancelOrders := base
		cancelOrders.Operation = BybitCancelOrders
		cancelConditional := base
		cancelConditional.Operation = BybitCancelConditional
		setLeverage := base
		setLeverage.Operation = BybitSetLeverage
		setLeverage.Leverage = decision.Leverage
		entry := base
		entry.Operation = BybitPlaceOrder
		entry.Side = entrySide
		entry.OrderType = "Market"
		entry.Quantity = quantity
		entry.ReferencePrice = decision.MarketPrice
		stop := base
		stop.Operation = BybitPlaceStopLoss
		stop.Side = exitSide
		stop.OrderType = "Market"
		stop.Quantity = quantity
		stop.ReduceOnly = true
		stop.TriggerPrice = decision.StopLoss
		stop.TriggerDirection = stopDirection
		take := base
		take.Operation = BybitPlaceTakeProfit
		take.Side = exitSide
		take.OrderType = "Market"
		take.Quantity = quantity
		take.ReduceOnly = true
		take.TriggerPrice = decision.TakeProfit
		take.TriggerDirection = takeDirection
		return []BybitOrderRequest{cancelOrders, cancelConditional, setLeverage, entry, stop, take}, nil

	case ActionCloseLong, ActionCloseShort:
		if decision.Quantity <= 0 {
			return nil, fmt.Errorf("close decision requires positive quantity")
		}
		closeRequest := base
		closeRequest.Operation = BybitPlaceOrder
		closeRequest.OrderType = "Market"
		closeRequest.Quantity = decision.Quantity
		closeRequest.ReduceOnly = true
		closeRequest.Side = "Sell"
		if decision.Action == ActionCloseShort {
			closeRequest.Side = "Buy"
		}
		return []BybitOrderRequest{closeRequest}, nil

	case ActionHold, ActionWait:
		return []BybitOrderRequest{}, nil
	default:
		return nil, fmt.Errorf("unsupported action %q", decision.Action)
	}
}

// IsIdempotentBybitSuccess recognizes responses where Bybit already has the
// requested final state. Position-mode mismatches are intentionally excluded.
func IsIdempotentBybitSuccess(response BybitResponse) bool {
	switch response.Code {
	case 0, 34040, 110043, 110026:
		return true
	default:
		return false
	}
}

// ReconcileBybitFill always prefers exchange-reported values over requested
// quantity/reference price.
func ReconcileBybitFill(request BybitOrderRequest, fill BybitExchangeFill) ReconciledFill {
	return ReconciledFill{
		OrderID:      fill.OrderID,
		Symbol:       request.Symbol,
		Quantity:     fill.Quantity,
		AveragePrice: fill.AveragePrice,
		Fee:          fill.Fee,
	}
}

// ShadowExecution records intended requests but has no HTTP client or private
// exchange transport, making accidental live submission impossible.
type ShadowExecution struct {
	mu       sync.RWMutex
	requests []BybitOrderRequest
}

func NewShadowExecution() *ShadowExecution {
	return &ShadowExecution{}
}

func (s *ShadowExecution) Execute(ctx context.Context, decision DecisionInput) (ExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return ExecutionResult{}, err
	}
	requests, err := BuildBybitExecutionPlan(decision)
	if err != nil {
		return ExecutionResult{}, err
	}
	s.mu.Lock()
	s.requests = append(s.requests, requests...)
	s.mu.Unlock()
	return ExecutionResult{Simulated: true, Requests: append([]BybitOrderRequest(nil), requests...)}, nil
}

func (s *ShadowExecution) Requests() []BybitOrderRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]BybitOrderRequest(nil), s.requests...)
}

func (s *ShadowExecution) LiveRequests() int { return 0 }
