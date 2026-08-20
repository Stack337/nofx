package parity

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestContextFetchesPositionMarketsBeforeCandidates(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 20, 8, 0, 0, 0, time.UTC)
	provider := &fakeContextProvider{
		account:   AccountSnapshot{TotalEquity: 100, AvailableBalance: 80},
		positions: []PositionSnapshot{{Symbol: "BTC-USDT", Side: "long"}},
		candidates: []CandidateSnapshot{
			{Symbol: "sol", Score: 90},
			{Symbol: "SOL-USDT", Score: 80},
			{Symbol: "bad", Score: 70},
		},
		markets: map[string]MarketSnapshot{
			"BTCUSDT": validMarket("BTCUSDT", 60000, now),
			"SOLUSDT": validMarket("SOLUSDT", 150, now),
			"BADUSDT": {Symbol: "BADUSDT", CurrentPrice: 0, PriceTimestamp: now},
		},
	}

	snapshot, err := BuildTradingContext(context.Background(), provider, now)
	if err != nil {
		t.Fatalf("build context: %v", err)
	}
	wantEvents := []string{"account", "positions", "candidates", "market:BTCUSDT", "market:SOLUSDT", "market:BADUSDT"}
	if !reflect.DeepEqual(provider.events, wantEvents) {
		t.Fatalf("events = %#v, want %#v", provider.events, wantEvents)
	}
	if len(snapshot.Candidates) != 1 || snapshot.Candidates[0].Symbol != "SOLUSDT" || snapshot.Candidates[0].Score != 90 {
		t.Fatalf("candidates = %+v", snapshot.Candidates)
	}
	if snapshot.Positions[0].Symbol != "BTCUSDT" {
		t.Fatalf("positions = %+v", snapshot.Positions)
	}
}

func TestContextMarksLiveFormingCandles(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 20, 8, 0, 0, 0, time.UTC)
	market := validMarket("SOLUSDT", 150, now)
	market.Candles = map[string][]CandleSnapshot{
		"15m": {
			{OpenTime: now.Add(-15 * time.Minute), CloseTime: now, Close: 149},
			{OpenTime: now, CloseTime: now.Add(15 * time.Minute), Close: 150},
		},
	}
	provider := &fakeContextProvider{
		candidates: []CandidateSnapshot{{Symbol: "SOLUSDT", Score: 90}},
		markets:    map[string]MarketSnapshot{"SOLUSDT": market},
	}

	snapshot, err := BuildTradingContext(context.Background(), provider, now)
	if err != nil {
		t.Fatalf("build context: %v", err)
	}
	candles := snapshot.Markets["SOLUSDT"].Candles["15m"]
	if candles[0].Forming || !candles[1].Forming {
		t.Fatalf("forming flags = [%v, %v]", candles[0].Forming, candles[1].Forming)
	}
}

func TestContextRetainsPositionWhenItsMarketFetchFails(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	provider := &fakeContextProvider{
		positions:    []PositionSnapshot{{Symbol: "BTCUSDT", Side: "long"}},
		marketErrors: map[string]error{"BTCUSDT": errors.New("provider unavailable")},
	}

	snapshot, err := BuildTradingContext(context.Background(), provider, now)
	if err != nil {
		t.Fatalf("build context: %v", err)
	}
	if len(snapshot.Positions) != 1 || snapshot.MarketHealth["BTCUSDT"].Healthy {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if snapshot.MarketHealth["BTCUSDT"].ErrorCode != "market_fetch_failed" {
		t.Fatalf("health = %+v", snapshot.MarketHealth["BTCUSDT"])
	}
}

func TestNormalizeCandidatesPreservesDirectionalFieldsFromHighestScore(t *testing.T) {
	got := normalizeCandidates([]CandidateSnapshot{
		{Symbol: "sol-usdt", Score: 71, LongScore: 20, ShortScore: 80, Confidence: 61, Regime: "bearish", ObservationID: "old", Sources: []string{"exchange"}},
		{Symbol: "SOLUSDT", Score: 84, LongScore: 90, ShortScore: 30, Confidence: 88, Regime: "bullish", ObservationID: "new", Sources: []string{"ai500"}},
	})
	want := []CandidateSnapshot{{
		Symbol: "SOLUSDT", Score: 84, LongScore: 90, ShortScore: 30, Confidence: 88,
		Regime: "bullish", ObservationID: "new", Sources: []string{"exchange", "ai500"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized candidates = %#v, want %#v", got, want)
	}
}

func validMarket(symbol string, price float64, now time.Time) MarketSnapshot {
	return MarketSnapshot{
		Symbol:         symbol,
		CurrentPrice:   price,
		MarkPrice:      price,
		PriceTimestamp: now,
		Candles: map[string][]CandleSnapshot{
			"15m": {{OpenTime: now.Add(-15 * time.Minute), CloseTime: now, Close: price}},
		},
	}
}

type fakeContextProvider struct {
	account      AccountSnapshot
	positions    []PositionSnapshot
	candidates   []CandidateSnapshot
	markets      map[string]MarketSnapshot
	marketErrors map[string]error
	events       []string
}

func (f *fakeContextProvider) Account(context.Context) (AccountSnapshot, error) {
	f.events = append(f.events, "account")
	return f.account, nil
}

func (f *fakeContextProvider) Positions(context.Context) ([]PositionSnapshot, error) {
	f.events = append(f.events, "positions")
	return append([]PositionSnapshot(nil), f.positions...), nil
}

func (f *fakeContextProvider) Candidates(context.Context) ([]CandidateSnapshot, error) {
	f.events = append(f.events, "candidates")
	return append([]CandidateSnapshot(nil), f.candidates...), nil
}

func (f *fakeContextProvider) Market(_ context.Context, symbol string) (MarketSnapshot, error) {
	f.events = append(f.events, "market:"+symbol)
	if err := f.marketErrors[symbol]; err != nil {
		return MarketSnapshot{}, err
	}
	return f.markets[symbol], nil
}
