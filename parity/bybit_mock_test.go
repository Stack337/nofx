package parity

import (
	"context"
	"testing"
)

func TestBybitContractOpenLongUsesOneWayMarketAndConditionalProtection(t *testing.T) {
	t.Parallel()

	decision := DecisionInput{
		Symbol:          "SOLUSDT",
		Action:          ActionOpenLong,
		Leverage:        3,
		PositionSizeUSD: 150,
		MarketPrice:     100,
		StopLoss:        98,
		TakeProfit:      106,
	}
	requests, err := BuildBybitExecutionPlan(decision)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if len(requests) != 6 {
		t.Fatalf("request count = %d, want 6: %+v", len(requests), requests)
	}
	if requests[0].Operation != BybitCancelOrders || requests[1].Operation != BybitCancelConditional || requests[2].Operation != BybitSetLeverage {
		t.Fatalf("cleanup/leverage sequence = %+v", requests[:3])
	}

	entry := requests[3]
	if entry.Operation != BybitPlaceOrder || entry.Category != "linear" || entry.OrderType != "Market" || entry.Side != "Buy" || entry.PositionIdx != 0 || entry.ReduceOnly {
		t.Fatalf("entry request = %+v", entry)
	}
	if entry.Quantity != 1.5 {
		t.Fatalf("entry quantity = %v, want 1.5", entry.Quantity)
	}

	stop, take := requests[4], requests[5]
	if stop.Operation != BybitPlaceStopLoss || !stop.ReduceOnly || stop.Side != "Sell" || stop.TriggerPrice != 98 || stop.PositionIdx != 0 {
		t.Fatalf("stop request = %+v", stop)
	}
	if take.Operation != BybitPlaceTakeProfit || !take.ReduceOnly || take.Side != "Sell" || take.TriggerPrice != 106 || take.PositionIdx != 0 {
		t.Fatalf("take request = %+v", take)
	}
}

func TestBybitContractCloseShortIsReduceOnly(t *testing.T) {
	t.Parallel()

	requests, err := BuildBybitExecutionPlan(DecisionInput{
		Symbol:   "BTCUSDT",
		Action:   ActionCloseShort,
		Quantity: 0.002,
	})
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("requests = %+v", requests)
	}
	closeRequest := requests[0]
	if closeRequest.Side != "Buy" || !closeRequest.ReduceOnly || closeRequest.PositionIdx != 0 || closeRequest.OrderType != "Market" {
		t.Fatalf("close request = %+v", closeRequest)
	}
}

func TestBybitContractTreatsAlreadyDesiredStateAsIdempotent(t *testing.T) {
	t.Parallel()

	for _, response := range []BybitResponse{
		{Code: 34040, Message: "not modified"},
		{Code: 110043, Message: "leverage not modified"},
		{Code: 0, Message: "OK"},
	} {
		if !IsIdempotentBybitSuccess(response) {
			t.Fatalf("response should be idempotent success: %+v", response)
		}
	}
	if IsIdempotentBybitSuccess(BybitResponse{Code: 10001, Message: "position idx not match position mode"}) {
		t.Fatal("position-mode mismatch must not be hidden")
	}
}

func TestBybitContractUsesActualExchangeFill(t *testing.T) {
	t.Parallel()

	result := ReconcileBybitFill(
		BybitOrderRequest{Symbol: "SOLUSDT", Quantity: 1.5, ReferencePrice: 100},
		BybitExchangeFill{OrderID: "order-1", Quantity: 1.49, AveragePrice: 100.25, Fee: 0.07},
	)
	if result.OrderID != "order-1" || result.Quantity != 1.49 || result.AveragePrice != 100.25 || result.Fee != 0.07 {
		t.Fatalf("reconciled fill = %+v", result)
	}
}

func TestBybitContractShadowExecutorCannotSendLiveRequest(t *testing.T) {
	t.Parallel()

	executor := NewShadowExecution()
	result, err := executor.Execute(context.Background(), DecisionInput{
		Symbol:          "SOLUSDT",
		Action:          ActionOpenLong,
		Leverage:        3,
		PositionSizeUSD: 150,
		MarketPrice:     100,
		StopLoss:        98,
		TakeProfit:      106,
	})
	if err != nil {
		t.Fatalf("shadow execute: %v", err)
	}
	if !result.Simulated || executor.LiveRequests() != 0 || len(executor.Requests()) != 6 {
		t.Fatalf("result=%+v live=%d requests=%+v", result, executor.LiveRequests(), executor.Requests())
	}
}
