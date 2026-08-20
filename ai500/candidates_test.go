package ai500

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"nofx/parity"
)

func TestCandidateProviderMapsLatestObservations(t *testing.T) {
	now := time.Date(2026, time.August, 20, 8, 0, 0, 0, time.UTC)
	store := &candidateStore{observations: map[string]ScoreObservation{
		"SOLUSDT": validCandidateObservation("SOLUSDT", 82, 71, 63, 77, now.Add(-2*time.Minute)),
	}}
	reader := NewCandidateProviderWithConfig(store, []string{" sol-usdt "}, CandidateProviderConfig{
		Now: func() time.Time { return now }, TTL: 10 * time.Minute, MinScore: 60, MinConfidence: 70,
	})

	got, err := reader(context.Background())
	if err != nil {
		t.Fatalf("read candidates: %v", err)
	}
	want := []parity.CandidateSnapshot{{
		Symbol: "SOLUSDT", Score: 82, LongScore: 71, ShortScore: 63,
		Confidence: 77, Regime: string(RegimeBullish), ObservationID: "obs-1", Sources: []string{"ai500"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %#v, want %#v", got, want)
	}
}

func TestCandidateProviderExcludesMissingStaleAndBelowThreshold(t *testing.T) {
	now := time.Date(2026, time.August, 20, 8, 0, 0, 0, time.UTC)
	store := &candidateStore{observations: map[string]ScoreObservation{
		"STALEUSDT": validCandidateObservation("STALEUSDT", 90, 80, 20, 90, now.Add(-11*time.Minute)),
		"LOWUSDT":   validCandidateObservation("LOWUSDT", 59, 80, 20, 90, now.Add(-time.Minute)),
	}}
	reader := NewCandidateProviderWithConfig(store, []string{"missing", "stale", "low"}, CandidateProviderConfig{
		Now: func() time.Time { return now }, TTL: 10 * time.Minute, MinScore: 60, MinConfidence: 80,
	})

	got, err := reader(context.Background())
	if err != nil {
		t.Fatalf("read candidates: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("candidates = %#v, want none", got)
	}
}

func TestCandidateProviderPropagatesStoreErrors(t *testing.T) {
	wantErr := errors.New("store unavailable")
	reader := NewCandidateProviderWithConfig(&candidateStore{err: wantErr}, []string{"SOLUSDT"}, CandidateProviderConfig{
		Now: time.Now, TTL: time.Hour,
	})

	_, err := reader(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

func validCandidateObservation(symbol string, score, longScore, shortScore, confidence float64, featureTime time.Time) ScoreObservation {
	return ScoreObservation{
		ID: "obs-1", Symbol: symbol, Score: score, LongScore: longScore, ShortScore: shortScore,
		Confidence: confidence, Regime: RegimeBullish, FeatureTimestamp: featureTime,
		CapturedAt: featureTime.Add(time.Minute), Provider: "test", Model: "test-model",
		PromptVersion: "p1", SchemaVersion: "s1",
	}
}

type candidateStore struct {
	observations map[string]ScoreObservation
	err          error
}

func (s *candidateStore) Put(context.Context, ScoreObservation) error { return nil }

func (s *candidateStore) GetLatest(_ context.Context, symbol string) (ScoreObservation, error) {
	if s.err != nil {
		return ScoreObservation{}, s.err
	}
	observation, ok := s.observations[symbol]
	if !ok {
		return ScoreObservation{}, ErrObservationNotFound
	}
	return observation, nil
}

func (s *candidateStore) AddOutcome(context.Context, OutcomeLabel) error { return nil }
