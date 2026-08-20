package ai500

import (
	"context"
	"math"
	"testing"
	"time"

	"nofx/parity"
)

func TestMarketLabelerSelectsCompletedHorizonsExactly(t *testing.T) {
	base := time.Date(2026, time.August, 20, 8, 0, 0, 0, time.UTC)
	observation := ScoreObservation{ID: "obs-long", Symbol: "sol-usdt", LongScore: 80, ShortScore: 20, FeatureTimestamp: base}
	market := parity.MarketSnapshot{Symbol: "SOLUSDT", Candles: map[string][]parity.CandleSnapshot{
		"15m": {
			labelCandle(base, 100, 100, 100, false),
			labelCandle(base.Add(15*time.Minute), 102, 99, 102, false),
			labelCandle(base.Add(time.Hour), 104, 98, 104, false),
			labelCandle(base.Add(4*time.Hour), 110, 97, 108, false),
			labelCandle(base.Add(24*time.Hour), 120, 90, 115, true),
		},
	}}

	labels, err := NewMarketLabeler().Label(context.Background(), observation, map[string]parity.MarketSnapshot{"SOLUSDT": market})
	if err != nil {
		t.Fatalf("label observation: %v", err)
	}
	if len(labels) != 3 {
		t.Fatalf("labels = %#v, want completed 15m/1h/4h only", labels)
	}
	assertLabel(t, labels[0], "15m", 2, -1)
	assertLabel(t, labels[1], "1h", 4, -2)
	assertLabel(t, labels[2], "4h", 8, -3)
	if !labels[2].LabeledAt.Equal(base.Add(4 * time.Hour)) {
		t.Fatalf("labeled_at = %v", labels[2].LabeledAt)
	}
}

func TestMarketLabelerUsesShortAdverseExcursion(t *testing.T) {
	base := time.Date(2026, time.August, 20, 8, 0, 0, 0, time.UTC)
	observation := ScoreObservation{ID: "obs-short", Symbol: "BTCUSDT", LongScore: 20, ShortScore: 80, FeatureTimestamp: base}
	market := parity.MarketSnapshot{Candles: map[string][]parity.CandleSnapshot{"15m": {
		labelCandle(base, 100, 100, 100, false),
		labelCandle(base.Add(15*time.Minute), 105, 98, 96, false),
	}}}

	labels, err := NewMarketLabeler().Label(context.Background(), observation, map[string]parity.MarketSnapshot{"btcusdt": market})
	if err != nil {
		t.Fatalf("label observation: %v", err)
	}
	if len(labels) != 1 {
		t.Fatalf("labels = %#v, want one", labels)
	}
	assertLabel(t, labels[0], "15m", -4, -5)
}

func TestMarketLabelerRequiresBaselineButOmitsMissingFutureCandles(t *testing.T) {
	base := time.Date(2026, time.August, 20, 8, 0, 0, 0, time.UTC)
	observation := ScoreObservation{ID: "obs", Symbol: "SOLUSDT", LongScore: 60, FeatureTimestamp: base}
	market := parity.MarketSnapshot{Candles: map[string][]parity.CandleSnapshot{"15m": {
		labelCandle(base, 100, 100, 100, false),
	}}}
	labels, err := NewMarketLabeler().Label(context.Background(), observation, map[string]parity.MarketSnapshot{"SOLUSDT": market})
	if err != nil || len(labels) != 0 {
		t.Fatalf("labels = %#v, error = %v, want empty successful result", labels, err)
	}

	market.Candles["15m"][0].CloseTime = base.Add(time.Minute)
	if _, err := NewMarketLabeler().Label(context.Background(), observation, map[string]parity.MarketSnapshot{"SOLUSDT": market}); err == nil {
		t.Fatal("missing baseline was accepted")
	}
}

func TestMarketLabelerDoesNotSubstituteLaterCandleForMissingHorizon(t *testing.T) {
	base := time.Date(2026, time.August, 20, 8, 0, 0, 0, time.UTC)
	observation := ScoreObservation{ID: "obs", Symbol: "SOLUSDT", LongScore: 60, FeatureTimestamp: base}
	market := parity.MarketSnapshot{Candles: map[string][]parity.CandleSnapshot{"15m": {
		labelCandle(base, 100, 100, 100, false),
		labelCandle(base.Add(time.Hour), 102, 99, 102, false),
	}}}

	labels, err := NewMarketLabeler().Label(context.Background(), observation, map[string]parity.MarketSnapshot{"SOLUSDT": market})
	if err != nil {
		t.Fatalf("label observation: %v", err)
	}
	if len(labels) != 1 || labels[0].Horizon != "1h" {
		t.Fatalf("labels = %#v, want only exact 1h horizon", labels)
	}
}

func labelCandle(closeTime time.Time, high, low, close float64, forming bool) parity.CandleSnapshot {
	return parity.CandleSnapshot{CloseTime: closeTime, High: high, Low: low, Close: close, Forming: forming}
}

func assertLabel(t *testing.T, got OutcomeLabel, horizon string, returnPct, drawdownPct float64) {
	t.Helper()
	if got.Horizon != horizon || math.Abs(got.ReturnPct-returnPct) > 1e-9 || math.Abs(got.MaxDrawdownPct-drawdownPct) > 1e-9 {
		t.Fatalf("label = %+v, want horizon=%s return=%v drawdown=%v", got, horizon, returnPct, drawdownPct)
	}
}
