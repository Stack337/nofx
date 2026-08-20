package parity

import (
	"testing"
	"time"
)

func defaultRiskConfig() RiskConfig {
	return RiskConfig{
		BTCETHMaxLeverage:       10,
		AltcoinMaxLeverage:      5,
		BTCETHMaxPositionRatio:  5,
		AltcoinMaxPositionRatio: 1,
		MinPositionGeneralUSD:   12,
		MinPositionBTCETHUSD:    60,
		MinRiskReward:           3,
		MaxPriceAge:             2 * time.Minute,
		MaxPriceDivergencePct:   0.5,
	}
}

func validLong(now time.Time) DecisionInput {
	return DecisionInput{
		Symbol:             "SOLUSDT",
		Action:             ActionOpenLong,
		Leverage:           3,
		PositionSizeUSD:    50,
		StopLoss:           98,
		TakeProfit:         106,
		MarketPrice:        100,
		PriceTimestamp:     now,
		PriceDivergencePct: 0.1,
	}
}

func TestRiskAcceptsValidOpen(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 20, 6, 0, 0, 0, time.UTC)
	result := ValidateDecision(validLong(now), 100, now, defaultRiskConfig())
	if !result.Allowed || result.ReasonCode != "" {
		t.Fatalf("valid open rejected: %+v", result)
	}
}

func TestRiskClampsLeverageToSymbolTier(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	decision := validLong(now)
	decision.Leverage = 20
	result := ValidateDecision(decision, 100, now, defaultRiskConfig())
	if !result.Allowed || result.Decision.Leverage != 5 {
		t.Fatalf("altcoin leverage result = %+v", result)
	}
	if len(result.Adjustments) != 1 || result.Adjustments[0] != "leverage_clamped" {
		t.Fatalf("adjustments = %#v", result.Adjustments)
	}
}

func TestRiskRejectsInvalidOpenInputs(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	tests := []struct {
		name   string
		mutate func(*DecisionInput)
		code   string
	}{
		{name: "bad stop ordering", mutate: func(d *DecisionInput) { d.StopLoss = 101 }, code: "invalid_sl_tp"},
		{name: "below minimum", mutate: func(d *DecisionInput) { d.PositionSizeUSD = 10 }, code: "minimum_order_value"},
		{name: "position cap", mutate: func(d *DecisionInput) { d.PositionSizeUSD = 102 }, code: "position_cap"},
		{name: "risk reward", mutate: func(d *DecisionInput) { d.TakeProfit = 104 }, code: "risk_reward_too_low"},
		{name: "stale price", mutate: func(d *DecisionInput) { d.PriceTimestamp = now.Add(-3 * time.Minute) }, code: "stale_price"},
		{name: "divergent price", mutate: func(d *DecisionInput) { d.PriceDivergencePct = 0.6 }, code: "divergent_price"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			decision := validLong(now)
			tt.mutate(&decision)
			result := ValidateDecision(decision, 100, now, defaultRiskConfig())
			if result.Allowed || result.ReasonCode != tt.code {
				t.Fatalf("result = %+v, want rejection %q", result, tt.code)
			}
		})
	}
}

func TestRiskUsesHigherBTCMinimum(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	decision := validLong(now)
	decision.Symbol = "BTCUSDT"
	decision.PositionSizeUSD = 59
	result := ValidateDecision(decision, 100, now, defaultRiskConfig())
	if result.Allowed || result.ReasonCode != "minimum_order_value" {
		t.Fatalf("result = %+v", result)
	}
}

func TestRiskAllowsCloseWhenPriceIsDegraded(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	decision := DecisionInput{
		Symbol:             "SOLUSDT",
		Action:             ActionCloseLong,
		PriceTimestamp:     now.Add(-time.Hour),
		PriceDivergencePct: 99,
	}
	result := ValidateDecision(decision, 100, now, defaultRiskConfig())
	if !result.Allowed {
		t.Fatalf("degraded-price close rejected: %+v", result)
	}
}
