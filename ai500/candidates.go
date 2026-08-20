package ai500

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"nofx/market"
	"nofx/parity"
)

type CandidateProviderConfig struct {
	Now           func() time.Time
	TTL           time.Duration
	MinScore      float64
	MinConfidence float64
}

func NewCandidateProvider(store ObservationStore, symbols []string) parity.CandidateReader {
	return NewCandidateProviderWithConfig(store, symbols, CandidateProviderConfig{})
}

func NewCandidateProviderWithConfig(store ObservationStore, symbols []string, config CandidateProviderConfig) parity.CandidateReader {
	normalizedSymbols := normalizeCandidateSymbols(symbols)
	if config.TTL <= 0 {
		config.TTL = 10 * time.Minute
	}
	return func(ctx context.Context) ([]parity.CandidateSnapshot, error) {
		if store == nil {
			return nil, errors.New("AI500 observation store is unavailable")
		}
		now := time.Now().UTC()
		if config.Now != nil {
			now = config.Now().UTC()
		}
		result := make([]parity.CandidateSnapshot, 0, len(normalizedSymbols))
		for _, symbol := range normalizedSymbols {
			observation, err := store.GetLatest(ctx, symbol)
			if errors.Is(err, ErrObservationNotFound) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("load AI500 observation for %s: %w", symbol, err)
			}
			normalized, err := NormalizeScore(observation, ScoreValidationConfig{Now: func() time.Time { return now }, MaxFeatureAge: config.TTL})
			if err != nil || normalized.Score < config.MinScore || normalized.Confidence < config.MinConfidence {
				continue
			}
			result = append(result, parity.CandidateSnapshot{
				Symbol: normalized.Symbol, Score: normalized.Score,
				LongScore: normalized.LongScore, ShortScore: normalized.ShortScore,
				Confidence: normalized.Confidence, Regime: string(normalized.Regime),
				ObservationID: normalized.ID, Sources: []string{"ai500"},
			})
		}
		return result, nil
	}
}

func normalizeCandidateSymbols(symbols []string) []string {
	result := make([]string, 0, len(symbols))
	seen := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		symbol = market.Normalize(strings.TrimSpace(symbol))
		if symbol == "" {
			continue
		}
		if _, exists := seen[symbol]; exists {
			continue
		}
		seen[symbol] = struct{}{}
		result = append(result, symbol)
	}
	return result
}
