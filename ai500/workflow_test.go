package ai500

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeScoreModel struct {
	response ScoreResponse
	err      error
	request  ScoreRequest
}

func (f *fakeScoreModel) Score(_ context.Context, request ScoreRequest) (ScoreResponse, error) {
	f.request = request
	return f.response, f.err
}

func workflowFeature(now time.Time) FeatureSnapshot {
	return FeatureSnapshot{
		Symbol: "BTCUSDT", BuiltAt: now, SourceTimestamp: now.Add(-time.Minute),
		CurrentPrice: 100, DataComplete: true,
		RequiredTimeframes: []string{"5m", "15m", "1h", "4h"},
		Timeframes:         map[string]TimeframeFeatures{"1h": {Close: 100, RSI: 60}},
	}
}

func TestWorkflowCreatesValidatedObservation(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	model := &fakeScoreModel{response: ScoreResponse{
		RawJSON:  []byte(`{"score":82.5,"long_score":90,"short_score":20,"confidence":78,"regime":"bullish","reasons":["trend","volume"]}`),
		Provider: "openrouter", Model: "deepseek-v4-flash",
	}}
	workflow := NewWorkflow(model, validationConfig(now))
	observation, err := workflow.Score(context.Background(), workflowFeature(now))
	if err != nil {
		t.Fatalf("Score() error = %v", err)
	}
	if observation.Symbol != "BTCUSDT" || observation.Score != 82.5 || observation.LongScore != 90 {
		t.Fatalf("observation = %+v", observation)
	}
	if observation.Provider != "openrouter" || observation.Model != "deepseek-v4-flash" {
		t.Fatalf("model metadata missing: %+v", observation)
	}
	if model.request.PromptVersion != PromptVersionV1 || model.request.SchemaVersion != SchemaVersionV1 {
		t.Fatalf("versions missing from request: %+v", model.request)
	}
}

func TestWorkflowRejectsMalformedAndInvalidJSON(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	cases := [][]byte{
		[]byte(`{"score":`),
		[]byte(`{"score":101,"long_score":90,"short_score":20,"confidence":78,"regime":"bullish","reasons":["trend"]}`),
		[]byte(`{"score":80,"long_score":90,"short_score":20,"confidence":78,"regime":"sideways","reasons":["trend"]}`),
		[]byte(`{"score":80,"long_score":90,"short_score":20,"confidence":78,"regime":"bullish","reasons":["trend"],"secret":"x"}`),
	}
	for _, raw := range cases {
		model := &fakeScoreModel{response: ScoreResponse{RawJSON: raw, Provider: "openrouter", Model: "deepseek"}}
		_, err := NewWorkflow(model, validationConfig(now)).Score(context.Background(), workflowFeature(now))
		var scoreErr *ScoreError
		if !errors.As(err, &scoreErr) || scoreErr.Code != "score_invalid" {
			t.Fatalf("Score() error = %v, want score_invalid", err)
		}
	}
}

func TestWorkflowMapsProviderFailureWithoutResponseBody(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	model := &fakeScoreModel{err: context.DeadlineExceeded}
	_, err := NewWorkflow(model, validationConfig(now)).Score(context.Background(), workflowFeature(now))
	var scoreErr *ScoreError
	if !errors.As(err, &scoreErr) || scoreErr.Code != "score_upstream_error" {
		t.Fatalf("Score() error = %v, want score_upstream_error", err)
	}
	if scoreErr.RawResponse != nil {
		t.Fatal("provider response body leaked into error")
	}
}

func TestWorkflowRejectsIncompleteFeaturesBeforeProvider(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	model := &fakeScoreModel{}
	features := workflowFeature(now)
	features.DataComplete = false
	_, err := NewWorkflow(model, validationConfig(now)).Score(context.Background(), features)
	var scoreErr *ScoreError
	if !errors.As(err, &scoreErr) || scoreErr.Code != "score_unavailable" {
		t.Fatalf("Score() error = %v, want score_unavailable", err)
	}
	if model.request.Symbol != "" {
		t.Fatal("provider was called for incomplete features")
	}
}
