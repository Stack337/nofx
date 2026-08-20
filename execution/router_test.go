package execution

import (
	"context"
	"errors"
	"testing"

	"nofx/agent"
	"nofx/provider"
	"nofx/risk"
)

type fakeExchange struct {
	placeCalls, statusCalls, protectionCalls int
	lastRequest                              OrderRequest
	placeResult                              OrderResult
	statusResult                             OrderState
}

func (f *fakeExchange) Account(context.Context) (AccountState, error) { return AccountState{}, nil }
func (f *fakeExchange) Instrument(context.Context, string) (InstrumentSpec, error) {
	return InstrumentSpec{QtyStep: 0.001, PriceTick: 0.1}, nil
}
func (f *fakeExchange) PositionMode(context.Context, string) (PositionMode, error) {
	return PositionModeOneWay, nil
}
func (f *fakeExchange) Place(_ context.Context, request OrderRequest) (OrderResult, error) {
	f.placeCalls++
	f.lastRequest = request
	return f.placeResult, nil
}

func TestRouterPreservesLongShortSemanticsForClose(t *testing.T) {
	fake := &fakeExchange{placeResult: OrderResult{OrderID: "close-1", Status: OrderStatusFilled}}
	router := NewRouter(fake)
	decision := authorizedDecision(true)
	decision.Decision.Action = agent.DecisionClose
	decision.Decision.Side = "short"
	if _, err := router.Execute(context.Background(), agent.ModeLive, decision, "cycle-close"); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if fake.lastRequest.Side != "short" || !fake.lastRequest.ReduceOnly {
		t.Fatalf("request = %+v, want short reduce-only close", fake.lastRequest)
	}
}
func (f *fakeExchange) GetOrder(context.Context, string, string) (OrderState, error) {
	f.statusCalls++
	return f.statusResult, nil
}
func (f *fakeExchange) SetProtection(context.Context, ProtectionRequest) error {
	f.protectionCalls++
	return nil
}

func TestRouterShadowAndPaperNeverCallExchange(t *testing.T) {
	fake := &fakeExchange{placeResult: OrderResult{OrderID: "live-1", Status: OrderStatusNew}}
	router := NewRouter(fake)
	decision := authorizedDecision(true)

	shadow, err := router.Execute(context.Background(), agent.ModeShadow, decision, "cycle-shadow")
	if err != nil || shadow.Status != OrderStatusShadow {
		t.Fatalf("shadow = %+v, %v", shadow, err)
	}
	paper, err := router.Execute(context.Background(), agent.ModePaper, decision, "cycle-paper")
	if err != nil || paper.Status != OrderStatusPaper {
		t.Fatalf("paper = %+v, %v", paper, err)
	}
	if fake.placeCalls != 0 || fake.protectionCalls != 0 {
		t.Fatalf("exchange mutation calls = place %d protection %d", fake.placeCalls, fake.protectionCalls)
	}
}

func TestRouterLiveRequiresPerDecisionConfirmation(t *testing.T) {
	fake := &fakeExchange{placeResult: OrderResult{OrderID: "live-1", Status: OrderStatusNew}}
	router := NewRouter(fake)
	decision := authorizedDecision(false)
	_, err := router.Execute(context.Background(), agent.ModeLive, decision, "cycle-live")
	if !errors.Is(err, ErrLiveGate) {
		t.Fatalf("Execute() error = %v, want ErrLiveGate", err)
	}
	if fake.placeCalls != 0 {
		t.Fatalf("place calls = %d, want zero", fake.placeCalls)
	}
}

func TestRouterReconcilesUnknownOrderBeforeReturning(t *testing.T) {
	fake := &fakeExchange{
		placeResult:  OrderResult{OrderID: "live-1", Status: OrderStatusUnknown},
		statusResult: OrderState{OrderID: "live-1", Status: OrderStatusFilled},
	}
	router := NewRouter(fake)
	result, err := router.Execute(context.Background(), agent.ModeLive, authorizedDecision(true), "cycle-reconcile")
	if err != nil || result.Status != OrderStatusFilled || fake.statusCalls != 1 {
		t.Fatalf("result = %+v, err = %v, status calls = %d", result, err, fake.statusCalls)
	}
}

func TestRouterPreventsDuplicateMutationForSameCycleAction(t *testing.T) {
	fake := &fakeExchange{placeResult: OrderResult{OrderID: "live-1", Status: OrderStatusNew}}
	router := NewRouter(fake)
	decision := authorizedDecision(true)
	first, err := router.Execute(context.Background(), agent.ModeLive, decision, "cycle-idempotent")
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	second, err := router.Execute(context.Background(), agent.ModeLive, decision, "cycle-idempotent")
	if err != nil || second.OrderID != first.OrderID || fake.placeCalls != 1 {
		t.Fatalf("second = %+v, err = %v, place calls = %d", second, err, fake.placeCalls)
	}
}

func authorizedDecision(liveConfirmed bool) risk.AuthorizedDecision {
	stop, take := 59000.0, 62000.0
	return risk.AuthorizedDecision{
		Decision: provider.DecisionResponse{
			Action: agent.DecisionOpen, Symbol: "BTCUSDT", Side: "buy", Quantity: 0.1,
			Leverage: 3, StopLoss: &stop, TakeProfit: &take,
		}, Notional: 6000, LiveConfirmed: liveConfirmed,
	}
}
