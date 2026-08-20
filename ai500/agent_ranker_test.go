package ai500

import (
	"context"
	"testing"
	"time"
)

func TestAgentRankerFiltersStaleAndSortsDeterministically(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	ranker := AgentRanker{Now: func() time.Time { return now }}
	items := []ScoreObservation{
		{Symbol: "ETHUSDT", Score: 80, Confidence: 70, FeatureTimestamp: now.Add(-time.Minute)},
		{Symbol: "BTCUSDT", Score: 80, Confidence: 75, FeatureTimestamp: now.Add(-time.Minute)},
		{Symbol: "OLDUSDT", Score: 99, Confidence: 99, FeatureTimestamp: now.Add(-time.Hour)},
	}

	got, err := ranker.Rank(context.Background(), items, RankConfig{Limit: 2, MaxAge: 10 * time.Minute})
	if err != nil {
		t.Fatalf("Rank() error = %v", err)
	}
	if len(got) != 2 || got[0].Symbol != "BTCUSDT" || got[1].Symbol != "ETHUSDT" {
		t.Fatalf("Rank() = %+v", got)
	}
}
