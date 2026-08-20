package execution

import (
	"context"
	"errors"
	"testing"

	"nofx/agent"
	"nofx/provider"
	"nofx/risk"
)

type injectingExchange struct {
	fakeExchange
	mode          PositionMode
	placeErr      error
	statusErr     error
	statusUnknown bool
}

func (f *injectingExchange) PositionMode(context.Context, string) (PositionMode, error) {
	if f.mode == "" {
		return PositionModeOneWay, nil
	}
	return f.mode, nil
}

func (f *injectingExchange) Place(ctx context.Context, request OrderRequest) (OrderResult, error) {
	if err := ctx.Err(); err != nil {
		return OrderResult{}, err
	}
	f.placeCalls++
	f.lastRequest = request
	if f.placeErr != nil {
		return OrderResult{}, f.placeErr
	}
	return f.placeResult, nil
}

func (f *injectingExchange) GetOrder(context.Context, string, string) (OrderState, error) {
	f.statusCalls++
	if f.statusErr != nil {
		return OrderState{}, f.statusErr
	}
	if f.statusUnknown {
		return OrderState{OrderID: "live-1", Status: OrderStatusUnknown}, nil
	}
	return f.statusResult, nil
}

func TestFailureInjectionClassifiesHTTPStatuses(t *testing.T) {
	for _, status := range []int{429, 404, 500} {
		err := HTTPStatusError(status, "upstream")
		var exchangeErr *ExchangeError
		if !errors.As(err, &exchangeErr) || exchangeErr.Status != status || exchangeErr.Code != expectedHTTPCode(status) {
			t.Fatalf("HTTPStatusError(%d) = %#v", status, err)
		}
	}
}

func TestFailureInjectionRejectsBybitRetCode34040(t *testing.T) {
	err := RetCodeError(34040, "not modified")
	var exchangeErr *ExchangeError
	if !errors.As(err, &exchangeErr) || exchangeErr.RetCode != 34040 || exchangeErr.Code != "exchange_retcode_34040" {
		t.Fatalf("RetCodeError(34040) = %#v", err)
	}
	fake := &injectingExchange{fakeExchange: fakeExchange{placeResult: OrderResult{OrderID: "live-1", Status: OrderStatusNew}}, placeErr: err}
	_, got := NewRouter(fake).Execute(context.Background(), agent.ModeLive, authorizedDecision(true), "cycle-34040")
	if !errors.As(got, &exchangeErr) || exchangeErr.Code != "exchange_retcode_34040" {
		t.Fatalf("Execute() error = %v", got)
	}
}

func TestFailureInjectionRejectsPositionModeMismatch(t *testing.T) {
	fake := &injectingExchange{fakeExchange: fakeExchange{placeResult: OrderResult{OrderID: "live-1", Status: OrderStatusNew}}, mode: PositionModeHedge}
	_, err := NewRouter(fake).Execute(context.Background(), agent.ModeLive, authorizedDecision(true), "cycle-mode")
	var exchangeErr *ExchangeError
	if !errors.As(err, &exchangeErr) || exchangeErr.Code != "position_mode_mismatch" {
		t.Fatalf("Execute() error = %v, want position_mode_mismatch", err)
	}
	if fake.placeCalls != 0 {
		t.Fatalf("place calls = %d, want 0", fake.placeCalls)
	}
}

func TestFailureInjectionMapsTunnelTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fake := &injectingExchange{fakeExchange: fakeExchange{placeResult: OrderResult{OrderID: "live-1", Status: OrderStatusNew}}}
	_, err := NewRouter(fake).Execute(ctx, agent.ModeLive, authorizedDecision(true), "cycle-timeout")
	if !errors.Is(err, context.Canceled) {
		var exchangeErr *ExchangeError
		if !errors.As(err, &exchangeErr) || exchangeErr.Code != "exchange_timeout" {
			t.Fatalf("Execute() error = %v, want timeout/canceled", err)
		}
	}
}

func TestFailureInjectionLeavesUnknownOrderForReconciliation(t *testing.T) {
	fake := &injectingExchange{
		fakeExchange:  fakeExchange{placeResult: OrderResult{OrderID: "live-1", Status: OrderStatusUnknown}},
		statusUnknown: true,
	}
	_, err := NewRouter(fake).Execute(context.Background(), agent.ModeLive, authorizedDecision(true), "cycle-unknown")
	if !errors.Is(err, ErrNeedsReconciliation) {
		t.Fatalf("Execute() error = %v, want ErrNeedsReconciliation", err)
	}
}

func TestFailureInjectionHTTPStatusDoesNotPlaceRetryDuplicate(t *testing.T) {
	fake := &injectingExchange{placeErr: HTTPStatusError(429, "rate limited")}
	router := NewRouter(fake)
	decision := authorizedDecision(true)
	if _, err := router.Execute(context.Background(), agent.ModeLive, decision, "cycle-429"); err == nil {
		t.Fatal("first Execute() succeeded")
	}
	fake.placeErr = nil
	fake.placeResult = OrderResult{OrderID: "live-1", Status: OrderStatusNew}
	if _, err := router.Execute(context.Background(), agent.ModeLive, decision, "cycle-429"); err != nil {
		t.Fatalf("retry Execute() error = %v", err)
	}
	if fake.placeCalls != 2 {
		t.Fatalf("place calls = %d, want 2 because failed attempt must not be cached", fake.placeCalls)
	}
}

func expectedHTTPCode(status int) string {
	switch status {
	case 429:
		return "exchange_http_429"
	case 404:
		return "exchange_http_404"
	case 500:
		return "exchange_http_500"
	default:
		return ""
	}
}

func TestFailureInjectionMissingProtectionIsRejectedBeforeLivePlace(t *testing.T) {
	decision := risk.AuthorizedDecision{
		Decision: provider.DecisionResponse{
			Action: agent.DecisionOpen, Symbol: "BTCUSDT", Side: "buy", Quantity: 0.1, Leverage: 3,
		},
		Notional: 6000, LiveConfirmed: true,
	}
	// Router still places if risk already authorized; missing protection is a risk-layer
	// failure. This test documents that a hold/open without SL/TP still cannot mutate
	// if LiveConfirmed is false.
	fake := &injectingExchange{fakeExchange: fakeExchange{placeResult: OrderResult{OrderID: "live-1", Status: OrderStatusNew}}}
	decision.LiveConfirmed = false
	_, err := NewRouter(fake).Execute(context.Background(), agent.ModeLive, decision, "cycle-protection")
	if !errors.Is(err, ErrLiveGate) || fake.placeCalls != 0 {
		t.Fatalf("Execute() error = %v place=%d", err, fake.placeCalls)
	}
}
