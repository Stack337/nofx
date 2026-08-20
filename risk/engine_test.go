package risk

import (
	"context"
	"errors"
	"testing"
	"time"

	"nofx/agent"
	"nofx/provider"
)

func TestEngineAuthorizesProtectedOpen(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	engine := NewEngine(func() time.Time { return now })
	decision, account, policy := validRiskInputs(now)

	got, err := engine.Authorize(context.Background(), decision, account, policy)
	if err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	if got.Notional != 6000 || got.Decision.Action != agent.DecisionOpen {
		t.Fatalf("authorized decision = %+v", got)
	}
}

func TestEngineRejectsHardLimitsWithStableCodes(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		mutate func(*provider.DecisionResponse, *AccountRisk, *Policy)
		code   string
	}{
		{"stale decision", func(d *provider.DecisionResponse, _ *AccountRisk, _ *Policy) {
			d.GeneratedAt = now.Add(-2 * time.Minute)
		}, CodeStaleDecision},
		{"kill switch", func(_ *provider.DecisionResponse, a *AccountRisk, _ *Policy) { a.KillSwitchEnabled = true }, CodeKillSwitch},
		{"leverage", func(d *provider.DecisionResponse, _ *AccountRisk, _ *Policy) { d.Leverage = 11 }, CodeMaxLeverage},
		{"notional", func(d *provider.DecisionResponse, _ *AccountRisk, _ *Policy) { d.Quantity = 1 }, CodeMaxNotional},
		{"daily loss", func(_ *provider.DecisionResponse, a *AccountRisk, _ *Policy) { a.DailyPnL = -101 }, CodeMaxDailyLoss},
		{"position count", func(_ *provider.DecisionResponse, a *AccountRisk, _ *Policy) { a.OpenPositions = 3 }, CodeMaxPositions},
		{"missing protection", func(d *provider.DecisionResponse, _ *AccountRisk, _ *Policy) { d.StopLoss = nil }, CodeProtectionRequired},
		{"unsafe stop", func(d *provider.DecisionResponse, _ *AccountRisk, _ *Policy) { value := 61000.0; d.StopLoss = &value }, CodeUnsafeProtection},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, account, policy := validRiskInputs(now)
			test.mutate(&decision, &account, &policy)
			_, err := NewEngine(func() time.Time { return now }).Authorize(context.Background(), decision, account, policy)
			var rejection *Rejection
			if !errors.As(err, &rejection) || rejection.Code != test.code {
				t.Fatalf("Authorize() error = %v, want code %s", err, test.code)
			}
		})
	}
}

func TestEngineAllowsHoldWhenKillSwitchIsEnabled(t *testing.T) {
	now := time.Now().UTC()
	decision, account, policy := validRiskInputs(now)
	decision.Action = agent.DecisionHold
	account.KillSwitchEnabled = true
	if _, err := NewEngine(func() time.Time { return now }).Authorize(context.Background(), decision, account, policy); err != nil {
		t.Fatalf("Authorize(hold) error = %v", err)
	}
}

func validRiskInputs(now time.Time) (provider.DecisionResponse, AccountRisk, Policy) {
	stop, take := 59000.0, 62000.0
	return provider.DecisionResponse{
			Action: agent.DecisionOpen, Symbol: "BTCUSDT", Side: "buy", Quantity: 0.1,
			Leverage: 3, StopLoss: &stop, TakeProfit: &take, Confidence: 80, GeneratedAt: now,
		}, AccountRisk{
			Equity: 1000, AvailableBalance: 500, DailyPnL: 0, OpenNotional: 0,
			OpenPositions: 1, MarkPrices: map[string]float64{"BTCUSDT": 60000},
		}, Policy{
			MaxLeverage: 10, MaxNotional: 10000, MaxDailyLoss: 100, MaxTradeLoss: 200,
			MaxPositions: 3, RequireProtection: true, MaxDecisionAge: time.Minute,
		}
}
