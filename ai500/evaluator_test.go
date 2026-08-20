package ai500

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestEvaluateCalculatesDirectionalMetricsAndTopK(t *testing.T) {
	base := time.Date(2026, time.August, 20, 8, 0, 0, 0, time.UTC)
	observations := []ScoreObservation{
		{ID: "a", Symbol: "AAAUSDT", Score: 90, LongScore: 80, ShortScore: 20, FeatureTimestamp: base},
		{ID: "b", Symbol: "BBBUSDT", Score: 80, LongScore: 20, ShortScore: 80, FeatureTimestamp: base},
		{ID: "c", Symbol: "CCCUSDT", Score: 70, LongScore: 70, ShortScore: 30, FeatureTimestamp: base},
	}
	labels := []OutcomeLabel{
		{ObservationID: "a", Horizon: "1h", ReturnPct: 5, MaxDrawdownPct: -2},
		{ObservationID: "b", Horizon: "1h", ReturnPct: 3, MaxDrawdownPct: -4},
		{ObservationID: "c", Horizon: "1h", ReturnPct: -1, MaxDrawdownPct: -1},
	}

	report := Evaluate(observations, labels, 2)
	if report.Count != 3 || math.Abs(report.PrecisionAtK-50) > 1e-9 || math.Abs(report.DirectionalAccuracy-100.0/3) > 1e-9 {
		t.Fatalf("report = %+v", report)
	}
	if math.Abs(report.AverageReturnPct-(5.0-3.0-1.0)/3.0) > 1e-9 || report.MaxDrawdownPct != -4 {
		t.Fatalf("return metrics = %+v", report)
	}
}

func TestEvaluateIsDeterministicAndMeasuresScoreStability(t *testing.T) {
	base := time.Date(2026, time.August, 20, 8, 0, 0, 0, time.UTC)
	observations := []ScoreObservation{
		{ID: "new", Symbol: "SOLUSDT", Score: 70, LongScore: 80, ShortScore: 20, FeatureTimestamp: base.Add(time.Hour)},
		{ID: "old", Symbol: "SOLUSDT", Score: 90, LongScore: 80, ShortScore: 20, FeatureTimestamp: base},
	}
	labels := []OutcomeLabel{
		{ObservationID: "new", Horizon: "1h", ReturnPct: 1},
		{ObservationID: "old", Horizon: "1h", ReturnPct: 1},
	}

	first := Evaluate(observations, labels, 10)
	second := Evaluate([]ScoreObservation{observations[1], observations[0]}, []OutcomeLabel{labels[1], labels[0]}, 10)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("reports differ: %+v vs %+v", first, second)
	}
	if first.ScoreStability != 80 {
		t.Fatalf("score stability = %v, want 80", first.ScoreStability)
	}
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("JSON differs: %s vs %s", firstJSON, secondJSON)
	}
}

func TestEvaluateIgnoresUnmatchedAndDuplicateLabels(t *testing.T) {
	observations := []ScoreObservation{{ID: "a", Symbol: "AAAUSDT", Score: 90, LongScore: 80}}
	labels := []OutcomeLabel{
		{ObservationID: "missing", Horizon: "1h", ReturnPct: 5},
		{ObservationID: "a", Horizon: "1h", ReturnPct: 2},
		{ObservationID: "a", Horizon: "1h", ReturnPct: -9},
	}
	report := Evaluate(observations, labels, 1)
	if report.Count != 1 || report.AverageReturnPct != 2 {
		t.Fatalf("report = %+v", report)
	}
}
