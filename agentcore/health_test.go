package agentcore

import (
	"context"
	"errors"
	"testing"

	"nofx/agent"
	"nofx/execution"
)

type healthExchange struct {
	accountErr error
	mode       execution.PositionMode
	modeErr    error
}

func (h healthExchange) Account(context.Context) (execution.AccountState, error) {
	return execution.AccountState{Equity: 1000, AvailableBalance: 500}, h.accountErr
}
func (h healthExchange) Instrument(context.Context, string) (execution.InstrumentSpec, error) {
	return execution.InstrumentSpec{QtyStep: 0.001}, nil
}
func (h healthExchange) PositionMode(context.Context, string) (execution.PositionMode, error) {
	if h.modeErr != nil {
		return "", h.modeErr
	}
	if h.mode == "" {
		return execution.PositionModeOneWay, nil
	}
	return h.mode, nil
}
func (healthExchange) Place(context.Context, execution.OrderRequest) (execution.OrderResult, error) {
	return execution.OrderResult{}, errors.New("place must not be called during health checks")
}
func (healthExchange) GetOrder(context.Context, string, string) (execution.OrderState, error) {
	return execution.OrderState{}, errors.New("get order must not be called during health checks")
}
func (healthExchange) SetProtection(context.Context, execution.ProtectionRequest) error {
	return errors.New("protection must not be called during health checks")
}

func TestExchangeHealthGateRejectsUnhealthyAccount(t *testing.T) {
	gate := ExchangeHealthGate{Exchange: healthExchange{accountErr: errors.New("balance unavailable")}}
	if err := gate.Ready(context.Background(), agent.Agent{ID: "agent-1"}); err == nil {
		t.Fatal("Ready() = nil, want account error")
	}
}

func TestExchangeHealthGateRejectsHedgeMode(t *testing.T) {
	gate := ExchangeHealthGate{Exchange: healthExchange{mode: execution.PositionModeHedge}}
	err := gate.Ready(context.Background(), agent.Agent{ID: "agent-1"})
	var exchangeErr *execution.ExchangeError
	if !errors.As(err, &exchangeErr) || exchangeErr.Code != "position_mode_mismatch" {
		t.Fatalf("Ready() error = %v", err)
	}
}

func TestExchangeHealthGatePassesOneWayAccount(t *testing.T) {
	gate := ExchangeHealthGate{Exchange: healthExchange{}}
	if err := gate.Ready(context.Background(), agent.Agent{ID: "agent-1"}); err != nil {
		t.Fatalf("Ready() error = %v", err)
	}
}
