package parity

import (
	"context"
	"testing"
	"time"

	"nofx/market"
)

func TestExchangeContextProviderReadsAccountPositionsAndMarket(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	reader := &fakeExchangeReader{
		balance:   map[string]any{"totalEquity": 100.0, "availableBalance": 80.0, "totalUnrealizedProfit": 5.0},
		positions: []map[string]any{{"symbol": "btcusdt", "side": "long", "positionAmt": 0.01, "entryPrice": 60000.0, "markPrice": 61000.0, "leverage": 3.0}},
		price:     61000,
	}
	candles := &fakeCandleReader{klines: []market.Kline{{OpenTime: now.Add(-time.Minute).UnixMilli(), CloseTime: now.UnixMilli(), Open: 60000, High: 61200, Low: 59900, Close: 61000, Volume: 10}}}
	provider := NewExchangeContextProvider(reader, candles, func(context.Context) ([]CandidateSnapshot, error) {
		return []CandidateSnapshot{{Symbol: "BTCUSDT", Score: 80}}, nil
	})

	account, err := provider.Account(context.Background())
	if err != nil || account.TotalEquity != 100 || account.PositionCount != 1 {
		t.Fatalf("account=%+v err=%v", account, err)
	}
	positions, err := provider.Positions(context.Background())
	if err != nil || len(positions) != 1 || positions[0].Symbol != "BTCUSDT" {
		t.Fatalf("positions=%+v err=%v", positions, err)
	}
	snapshot, err := provider.Market(context.Background(), "BTCUSDT")
	if err != nil || snapshot.CurrentPrice != 61000 || len(snapshot.Candles) == 0 {
		t.Fatalf("market=%+v err=%v", snapshot, err)
	}
}

type fakeExchangeReader struct {
	balance   map[string]any
	positions []map[string]any
	price     float64
}

func (f *fakeExchangeReader) GetBalance() (map[string]any, error)     { return f.balance, nil }
func (f *fakeExchangeReader) GetPositions() ([]map[string]any, error) { return f.positions, nil }
func (f *fakeExchangeReader) GetMarketPrice(string) (float64, error)  { return f.price, nil }

type fakeCandleReader struct{ klines []market.Kline }

func (f *fakeCandleReader) GetKlines(string, string, int) ([]market.Kline, error) {
	return f.klines, nil
}
