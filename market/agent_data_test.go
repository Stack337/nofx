package market

import (
	"errors"
	"testing"
	"time"
)

func TestNormalizeAgentSnapshotRejectsStaleCandles(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	raw := AgentMarketData{
		Symbol:  "btcusdt",
		Candles: []KlineBar{{Time: now.Add(-11 * time.Minute).UnixMilli(), Close: 60000, Volume: 10}},
	}
	_, err := NormalizeAgentSnapshot(now, raw, SourceConfig{MaxAge: 10 * time.Minute})
	if !errors.Is(err, ErrAgentMarketStale) {
		t.Fatalf("NormalizeAgentSnapshot() error = %v, want ErrAgentMarketStale", err)
	}
}

func TestNormalizeAgentSnapshotProducesFreshIndependentCopy(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	raw := AgentMarketData{
		Symbol: "btcusdt", Spread: 0.02, Funding: 0.0001, OpenInterest: 1200,
		Candles: []KlineBar{{Time: now.Add(-time.Minute).UnixMilli(), Close: 60000, Volume: 10}},
	}
	got, err := NormalizeAgentSnapshot(now, raw, SourceConfig{MaxAge: 10 * time.Minute})
	if err != nil {
		t.Fatalf("NormalizeAgentSnapshot() error = %v", err)
	}
	raw.Candles[0].Close = 1
	if got.Symbol != "BTCUSDT" || !got.Fresh || got.Candles[0].Close != 60000 || got.Volume != 10 {
		t.Fatalf("snapshot = %+v", got)
	}
}
