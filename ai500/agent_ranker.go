package ai500

import (
	"context"
	"sort"
	"time"
)

type Candidate struct {
	Symbol     string
	Score      float64
	LongScore  float64
	ShortScore float64
	Confidence float64
	Regime     Regime
	ObservedAt time.Time
	Reasons    []string
}

type RankConfig struct {
	Limit  int
	MaxAge time.Duration
}

type CandidateRanker interface {
	Rank(context.Context, []ScoreObservation, RankConfig) ([]Candidate, error)
}

type AgentRanker struct {
	Now func() time.Time
}

func (r AgentRanker) Rank(ctx context.Context, observations []ScoreObservation, config RankConfig) ([]Candidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if config.Limit <= 0 {
		config.Limit = 20
	}
	if config.MaxAge <= 0 {
		config.MaxAge = 10 * time.Minute
	}
	now := time.Now().UTC()
	if r.Now != nil {
		now = r.Now().UTC()
	}
	candidates := make([]Candidate, 0, len(observations))
	for _, observation := range observations {
		age := now.Sub(observation.FeatureTimestamp.UTC())
		if observation.Symbol == "" || age < -time.Second || age > config.MaxAge {
			continue
		}
		candidates = append(candidates, Candidate{
			Symbol: observation.Symbol, Score: observation.Score, LongScore: observation.LongScore,
			ShortScore: observation.ShortScore, Confidence: observation.Confidence,
			Regime: observation.Regime, ObservedAt: observation.FeatureTimestamp.UTC(),
			Reasons: append([]string(nil), observation.Reasons...),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		if candidates[i].Confidence != candidates[j].Confidence {
			return candidates[i].Confidence > candidates[j].Confidence
		}
		return candidates[i].Symbol < candidates[j].Symbol
	})
	if len(candidates) > config.Limit {
		candidates = candidates[:config.Limit]
	}
	return candidates, nil
}
