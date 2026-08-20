package parity

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"nofx/market"
)

type ExchangeReader interface {
	GetBalance() (map[string]any, error)
	GetPositions() ([]map[string]any, error)
	GetMarketPrice(string) (float64, error)
}

type CandleReader interface {
	GetKlines(symbol, interval string, limit int) ([]market.Kline, error)
}

type CandidateReader func(context.Context) ([]CandidateSnapshot, error)

type ExchangeContextProvider struct {
	exchange   ExchangeReader
	candles    CandleReader
	candidates CandidateReader
}

func NewExchangeContextProvider(exchange ExchangeReader, candles CandleReader, candidates CandidateReader) *ExchangeContextProvider {
	return &ExchangeContextProvider{exchange: exchange, candles: candles, candidates: candidates}
}

func (p *ExchangeContextProvider) Account(ctx context.Context) (AccountSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return AccountSnapshot{}, err
	}
	balance, err := p.exchange.GetBalance()
	if err != nil {
		return AccountSnapshot{}, err
	}
	positions, err := p.exchange.GetPositions()
	if err != nil {
		return AccountSnapshot{}, err
	}
	equity := mapFloat(balance, "totalEquity", "equity", "balance")
	available := mapFloat(balance, "availableBalance", "totalAvailableBalance")
	marginPercent := 0.0
	if equity > 0 {
		marginPercent = math.Max(0, (equity-available)/equity*100)
	}
	return AccountSnapshot{
		TotalEquity: equity, AvailableBalance: available,
		UnrealizedPnL: mapFloat(balance, "totalUnrealizedProfit", "unrealizedPnL"),
		PositionCount: len(positions), MarginUsedPercent: marginPercent,
	}, nil
}

func (p *ExchangeContextProvider) Positions(ctx context.Context) ([]PositionSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows, err := p.exchange.GetPositions()
	if err != nil {
		return nil, err
	}
	result := make([]PositionSnapshot, 0, len(rows))
	for _, row := range rows {
		quantity := mapFloat(row, "positionAmt", "quantity", "size")
		side := strings.ToLower(strings.TrimSpace(mapString(row, "side", "positionSide")))
		if quantity < 0 {
			quantity = math.Abs(quantity)
			if side == "" {
				side = "short"
			}
		}
		result = append(result, PositionSnapshot{
			Symbol: mapString(row, "symbol"), Side: side, Quantity: quantity,
			EntryPrice: mapFloat(row, "entryPrice", "avgPrice"), MarkPrice: mapFloat(row, "markPrice"),
			Leverage: int(mapFloat(row, "leverage")), UnrealizedPnL: mapFloat(row, "unRealizedProfit", "unrealizedPnL"),
			LiquidationPrice: mapFloat(row, "liquidationPrice", "liqPrice"),
		})
	}
	return normalizePositions(result), nil
}

func (p *ExchangeContextProvider) Candidates(ctx context.Context) ([]CandidateSnapshot, error) {
	if p.candidates == nil {
		return []CandidateSnapshot{}, nil
	}
	return p.candidates(ctx)
}

func (p *ExchangeContextProvider) Market(ctx context.Context, symbol string) (MarketSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return MarketSnapshot{}, err
	}
	price, err := p.exchange.GetMarketPrice(symbol)
	if err != nil {
		return MarketSnapshot{}, err
	}
	snapshot := MarketSnapshot{Symbol: symbol, CurrentPrice: price, MarkPrice: price, PriceTimestamp: time.Now().UTC(), Candles: map[string][]CandleSnapshot{}}
	for _, timeframe := range []string{"15m", "4h", "1m"} {
		klines, fetchErr := p.candles.GetKlines(symbol, timeframe, 30)
		if fetchErr != nil {
			return MarketSnapshot{}, fmt.Errorf("load %s candles: %w", timeframe, fetchErr)
		}
		converted := make([]CandleSnapshot, 0, len(klines))
		for _, kline := range klines {
			converted = append(converted, CandleSnapshot{
				OpenTime: time.UnixMilli(kline.OpenTime).UTC(), CloseTime: time.UnixMilli(kline.CloseTime).UTC(),
				Open: kline.Open, High: kline.High, Low: kline.Low, Close: kline.Close, Volume: kline.Volume,
			})
		}
		snapshot.Candles[timeframe] = converted
	}
	return snapshot, nil
}

func mapFloat(values map[string]any, keys ...string) float64 {
	for _, key := range keys {
		switch value := values[key].(type) {
		case float64:
			return value
		case float32:
			return float64(value)
		case int:
			return float64(value)
		case int64:
			return float64(value)
		case json.Number:
			parsed, _ := value.Float64()
			return parsed
		case string:
			parsed, _ := strconv.ParseFloat(value, 64)
			return parsed
		}
	}
	return 0
}

func mapString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok {
			return value
		}
	}
	return ""
}
