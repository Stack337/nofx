package parity

import (
	"context"
	"fmt"
	"time"

	"nofx/market"
)

type AccountSnapshot struct {
	TotalEquity       float64
	AvailableBalance  float64
	UnrealizedPnL     float64
	PositionCount     int
	MarginUsedPercent float64
}

type PositionSnapshot struct {
	Symbol           string
	Side             string
	Quantity         float64
	EntryPrice       float64
	MarkPrice        float64
	Leverage         int
	UnrealizedPnL    float64
	LiquidationPrice float64
}

type CandidateSnapshot struct {
	Symbol        string
	Score         float64
	LongScore     float64
	ShortScore    float64
	Confidence    float64
	Regime        string
	ObservationID string
	Sources       []string
}

type CandleSnapshot struct {
	OpenTime  time.Time
	CloseTime time.Time
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
	Forming   bool
}

type MarketSnapshot struct {
	Symbol             string
	CurrentPrice       float64
	MarkPrice          float64
	PriceTimestamp     time.Time
	PriceDivergencePct float64
	Candles            map[string][]CandleSnapshot
}

type MarketHealth struct {
	Healthy   bool
	ErrorCode string
}

type TradingContextSnapshot struct {
	CollectedAt  time.Time
	Account      AccountSnapshot
	Positions    []PositionSnapshot
	Candidates   []CandidateSnapshot
	Markets      map[string]MarketSnapshot
	MarketHealth map[string]MarketHealth
}

type ContextProvider interface {
	Account(context.Context) (AccountSnapshot, error)
	Positions(context.Context) ([]PositionSnapshot, error)
	Candidates(context.Context) ([]CandidateSnapshot, error)
	Market(context.Context, string) (MarketSnapshot, error)
}

// BuildTradingContext creates one immutable, timestamped input snapshot. It
// fetches position markets before candidate markets so risk management does not
// depend on candidate-provider health.
func BuildTradingContext(ctx context.Context, provider ContextProvider, now time.Time) (*TradingContextSnapshot, error) {
	account, err := provider.Account(ctx)
	if err != nil {
		return nil, fmt.Errorf("load account snapshot: %w", err)
	}
	positions, err := provider.Positions(ctx)
	if err != nil {
		return nil, fmt.Errorf("load position snapshots: %w", err)
	}
	candidates, err := provider.Candidates(ctx)
	if err != nil {
		return nil, fmt.Errorf("load candidate snapshots: %w", err)
	}

	snapshot := &TradingContextSnapshot{
		CollectedAt:  now.UTC(),
		Account:      account,
		Positions:    normalizePositions(positions),
		Markets:      map[string]MarketSnapshot{},
		MarketHealth: map[string]MarketHealth{},
	}
	normalizedCandidates := normalizeCandidates(candidates)

	fetched := map[string]bool{}
	for _, position := range snapshot.Positions {
		fetchContextMarket(ctx, provider, snapshot, position.Symbol, now)
		fetched[position.Symbol] = true
	}
	for _, candidate := range normalizedCandidates {
		if !fetched[candidate.Symbol] {
			fetchContextMarket(ctx, provider, snapshot, candidate.Symbol, now)
			fetched[candidate.Symbol] = true
		}
		if snapshot.MarketHealth[candidate.Symbol].Healthy {
			snapshot.Candidates = append(snapshot.Candidates, candidate)
		}
	}

	return snapshot, nil
}

func normalizePositions(positions []PositionSnapshot) []PositionSnapshot {
	result := make([]PositionSnapshot, 0, len(positions))
	seen := map[string]bool{}
	for _, position := range positions {
		position.Symbol = market.Normalize(position.Symbol)
		if position.Symbol == "" || seen[position.Symbol] {
			continue
		}
		seen[position.Symbol] = true
		result = append(result, position)
	}
	return result
}

func normalizeCandidates(candidates []CandidateSnapshot) []CandidateSnapshot {
	result := make([]CandidateSnapshot, 0, len(candidates))
	indexes := map[string]int{}
	for _, candidate := range candidates {
		candidate.Symbol = market.Normalize(candidate.Symbol)
		if candidate.Symbol == "" {
			continue
		}
		if index, exists := indexes[candidate.Symbol]; exists {
			mergedSources := mergeSources(result[index].Sources, candidate.Sources)
			if candidate.Score > result[index].Score {
				candidate.Sources = mergedSources
				result[index] = candidate
			} else {
				result[index].Sources = mergedSources
			}
			continue
		}
		indexes[candidate.Symbol] = len(result)
		candidate.Sources = mergeSources(nil, candidate.Sources)
		result = append(result, candidate)
	}
	return result
}

func mergeSources(existing, incoming []string) []string {
	result := append([]string(nil), existing...)
	seen := map[string]bool{}
	for _, source := range result {
		seen[source] = true
	}
	for _, source := range incoming {
		if source == "" || seen[source] {
			continue
		}
		seen[source] = true
		result = append(result, source)
	}
	return result
}

func fetchContextMarket(ctx context.Context, provider ContextProvider, snapshot *TradingContextSnapshot, symbol string, now time.Time) {
	marketSnapshot, err := provider.Market(ctx, symbol)
	if err != nil {
		snapshot.MarketHealth[symbol] = MarketHealth{ErrorCode: "market_fetch_failed"}
		return
	}
	marketSnapshot.Symbol = symbol
	markFormingCandles(&marketSnapshot, now)
	if marketSnapshot.CurrentPrice <= 0 || marketSnapshot.PriceTimestamp.IsZero() || len(marketSnapshot.Candles) == 0 {
		snapshot.MarketHealth[symbol] = MarketHealth{ErrorCode: "invalid_market_snapshot"}
		return
	}
	snapshot.Markets[symbol] = marketSnapshot
	snapshot.MarketHealth[symbol] = MarketHealth{Healthy: true}
}

func markFormingCandles(snapshot *MarketSnapshot, now time.Time) {
	for timeframe, candles := range snapshot.Candles {
		for index := range candles {
			candles[index].Forming = candles[index].CloseTime.After(now)
		}
		snapshot.Candles[timeframe] = candles
	}
}
