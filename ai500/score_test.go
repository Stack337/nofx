package ai500

import (
	"math"
	"strings"
	"testing"
	"time"
)

func validObservation(now time.Time) ScoreObservation {
	return ScoreObservation{
		Symbol:           " btcusdt ",
		Score:            82.5,
		LongScore:        90,
		ShortScore:       25,
		Confidence:       78,
		Regime:           RegimeBullish,
		Reasons:          []string{"trend", "volume"},
		FeatureTimestamp: now.Add(-2 * time.Minute),
		CapturedAt:       now,
		Provider:         "openrouter",
		Model:            "deepseek-v4-flash",
		PromptVersion:    "ai500-v1",
		SchemaVersion:    "ai500-score-v1",
	}
}

func validationConfig(now time.Time) ScoreValidationConfig {
	return ScoreValidationConfig{
		Now:            func() time.Time { return now },
		MaxFeatureAge:  10 * time.Minute,
		MaxReasons:     4,
		MaxReasonBytes: 128,
	}
}

func TestNormalizeScoreUppercasesAndCopiesReasons(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	original := validObservation(now)
	normalized, err := NormalizeScore(original, validationConfig(now))
	if err != nil {
		t.Fatalf("NormalizeScore() error = %v", err)
	}
	if normalized.Symbol != "BTCUSDT" {
		t.Fatalf("symbol = %q, want BTCUSDT", normalized.Symbol)
	}
	normalized.Reasons[0] = "changed"
	if original.Reasons[0] == "changed" {
		t.Fatal("NormalizeScore mutated the input reasons")
	}
}

func TestValidateScoreRejectsNonFiniteAndOutOfRangeNumbers(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		mutate func(*ScoreObservation)
	}{
		{"nan", func(o *ScoreObservation) { o.Score = math.NaN() }},
		{"infinity", func(o *ScoreObservation) { o.Confidence = math.Inf(1) }},
		{"below zero", func(o *ScoreObservation) { o.LongScore = -0.1 }},
		{"above one hundred", func(o *ScoreObservation) { o.ShortScore = 100.1 }},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			observation := validObservation(now)
			tt.mutate(&observation)
			if err := ValidateScore(observation, validationConfig(now)); err == nil {
				t.Fatal("ValidateScore() error = nil, want validation error")
			}
		})
	}
}

func TestValidateScoreRejectsInvalidMetadataAndStaleFeatures(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		mutate func(*ScoreObservation, *ScoreValidationConfig)
	}{
		{"empty symbol", func(o *ScoreObservation, _ *ScoreValidationConfig) { o.Symbol = "   " }},
		{"unknown regime", func(o *ScoreObservation, _ *ScoreValidationConfig) { o.Regime = "sideways" }},
		{"stale feature", func(o *ScoreObservation, c *ScoreValidationConfig) {
			o.FeatureTimestamp = c.Now().Add(-c.MaxFeatureAge - time.Second)
		}},
		{"missing prompt version", func(o *ScoreObservation, _ *ScoreValidationConfig) { o.PromptVersion = "" }},
		{"missing schema version", func(o *ScoreObservation, _ *ScoreValidationConfig) { o.SchemaVersion = "" }},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			observation := validObservation(now)
			config := validationConfig(now)
			tt.mutate(&observation, &config)
			if err := ValidateScore(observation, config); err == nil {
				t.Fatal("ValidateScore() error = nil, want validation error")
			}
		})
	}
}

func TestValidateScoreRejectsUnboundedReasons(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	observation := validObservation(now)
	config := validationConfig(now)
	observation.Reasons = make([]string, config.MaxReasons+1)
	if err := ValidateScore(observation, config); err == nil {
		t.Fatal("too many reasons were accepted")
	}

	observation = validObservation(now)
	observation.Reasons = []string{strings.Repeat("x", config.MaxReasonBytes+1)}
	if err := ValidateScore(observation, config); err == nil {
		t.Fatal("oversized reason was accepted")
	}
}
