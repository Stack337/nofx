package ai500

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestAI500FakeProviderPersistsCandidateForShadowConsumption(t *testing.T) {
	now := time.Date(2026, time.August, 20, 8, 0, 0, 0, time.UTC)
	workflow := NewWorkflow(e2eScoreModel{}, ScoreValidationConfig{Now: func() time.Time { return now }, MaxFeatureAge: time.Hour})
	observation, err := workflow.Score(context.Background(), FeatureSnapshot{
		Symbol: "SOLUSDT", BuiltAt: now, SourceTimestamp: now.Add(-time.Minute), CurrentPrice: 100,
		DataComplete: true, Timeframes: map[string]TimeframeFeatures{},
	})
	if err != nil {
		t.Fatalf("score features: %v", err)
	}
	store, err := NewJSONLStore(filepath.Join(t.TempDir(), "private", "observations.jsonl"))
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	if err := store.Put(context.Background(), observation); err != nil {
		t.Fatalf("persist observation: %v", err)
	}
	reader := NewCandidateProviderWithConfig(store, []string{"SOLUSDT"}, CandidateProviderConfig{Now: func() time.Time { return now }, TTL: time.Hour})
	candidates, err := reader(context.Background())
	if err != nil || len(candidates) != 1 {
		t.Fatalf("candidates = %#v, error = %v", candidates, err)
	}
	if candidates[0].Score != 91 || candidates[0].ObservationID == "" || len(candidates[0].Sources) != 1 || candidates[0].Sources[0] != "ai500" {
		t.Fatalf("candidate = %+v", candidates[0])
	}
}

type e2eScoreModel struct{}

func (e2eScoreModel) Score(context.Context, ScoreRequest) (ScoreResponse, error) {
	return ScoreResponse{
		RawJSON:  []byte(`{"score":91,"long_score":88,"short_score":22,"confidence":84,"regime":"bullish","reasons":["trend"]}`),
		Provider: "fake", Model: "fake-ai500",
	}, nil
}
