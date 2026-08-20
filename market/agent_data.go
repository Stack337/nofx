package market

import (
	"context"
	"errors"
	"math"
	"time"
)

var (
	ErrAgentMarketIncomplete = errors.New("agent market data is incomplete")
	ErrAgentMarketStale      = errors.New("agent market data is stale")
)

type Liquidation struct {
	Timestamp time.Time
	Side      string
	Price     float64
	Quantity  float64
}

type AgentMarketData struct {
	Symbol       string
	Candles      []KlineBar
	Spread       float64
	Funding      float64
	OpenInterest float64
	Liquidations []Liquidation
}

type MarketSnapshot struct {
	Symbol       string
	Timestamp    time.Time
	Candles      []KlineBar
	Volume       float64
	Spread       float64
	Funding      float64
	OpenInterest float64
	Liquidations []Liquidation
	Fresh        bool
}

type SourceConfig struct {
	MaxAge time.Duration
}

type MarketSource interface {
	Snapshot(context.Context, string, SourceConfig) (MarketSnapshot, error)
}

func NormalizeAgentSnapshot(now time.Time, raw AgentMarketData, config SourceConfig) (MarketSnapshot, error) {
	if config.MaxAge <= 0 {
		config.MaxAge = 10 * time.Minute
	}
	symbol := Normalize(raw.Symbol)
	if symbol == "" || len(raw.Candles) == 0 {
		return MarketSnapshot{}, ErrAgentMarketIncomplete
	}
	candles := append([]KlineBar(nil), raw.Candles...)
	last := candles[len(candles)-1]
	timestamp := time.UnixMilli(last.Time).UTC()
	if last.Time <= 0 || last.Close <= 0 || math.IsNaN(last.Close) || math.IsInf(last.Close, 0) {
		return MarketSnapshot{}, ErrAgentMarketIncomplete
	}
	now = now.UTC()
	if timestamp.After(now.Add(time.Second)) || now.Sub(timestamp) > config.MaxAge {
		return MarketSnapshot{}, ErrAgentMarketStale
	}
	liquidations := append([]Liquidation(nil), raw.Liquidations...)
	return MarketSnapshot{
		Symbol: symbol, Timestamp: timestamp, Candles: candles, Volume: last.Volume,
		Spread: raw.Spread, Funding: raw.Funding, OpenInterest: raw.OpenInterest,
		Liquidations: liquidations, Fresh: true,
	}, nil
}
