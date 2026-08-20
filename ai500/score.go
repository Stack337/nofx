package ai500

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

type Regime string

const (
	RegimeBullish Regime = "bullish"
	RegimeBearish Regime = "bearish"
	RegimeNeutral Regime = "neutral"
)

type ScoreObservation struct {
	ID               string    `json:"id,omitempty"`
	Symbol           string    `json:"symbol"`
	Score            float64   `json:"score"`
	LongScore        float64   `json:"long_score"`
	ShortScore       float64   `json:"short_score"`
	Confidence       float64   `json:"confidence"`
	Regime           Regime    `json:"regime"`
	Reasons          []string  `json:"reasons"`
	FeatureTimestamp time.Time `json:"feature_timestamp"`
	CapturedAt       time.Time `json:"captured_at"`
	Provider         string    `json:"model_provider"`
	Model            string    `json:"model_name"`
	PromptVersion    string    `json:"prompt_version"`
	SchemaVersion    string    `json:"schema_version"`
}

type ScoreValidationConfig struct {
	Now            func() time.Time
	MaxFeatureAge  time.Duration
	MaxReasons     int
	MaxReasonBytes int
}

// ScoreValidator is kept as a named type so callers can make validation a
// dependency in workflows without coupling themselves to implementation.
type ScoreValidator struct {
	Config ScoreValidationConfig
}

func ValidateScore(observation ScoreObservation, configs ...ScoreValidationConfig) error {
	_, err := normalizeScore(observation, configFrom(configs))
	return err
}

func NormalizeScore(observation ScoreObservation, configs ...ScoreValidationConfig) (ScoreObservation, error) {
	return normalizeScore(observation, configFrom(configs))
}

func (v ScoreValidator) Validate(observation ScoreObservation) error {
	return ValidateScore(observation, v.Config)
}

func normalizeScore(observation ScoreObservation, config ScoreValidationConfig) (ScoreObservation, error) {
	now := time.Now().UTC()
	if config.Now != nil {
		now = config.Now().UTC()
	}
	if config.MaxFeatureAge <= 0 {
		config.MaxFeatureAge = 10 * time.Minute
	}
	if config.MaxReasons <= 0 {
		config.MaxReasons = 8
	}
	if config.MaxReasonBytes <= 0 {
		config.MaxReasonBytes = 256
	}

	observation.Symbol = strings.ToUpper(strings.TrimSpace(observation.Symbol))
	observation.Provider = strings.TrimSpace(observation.Provider)
	observation.Model = strings.TrimSpace(observation.Model)
	observation.PromptVersion = strings.TrimSpace(observation.PromptVersion)
	observation.SchemaVersion = strings.TrimSpace(observation.SchemaVersion)
	observation.Reasons = append([]string(nil), observation.Reasons...)

	if observation.Symbol == "" {
		return ScoreObservation{}, errors.New("score symbol is required")
	}
	if observation.Regime != RegimeBullish && observation.Regime != RegimeBearish && observation.Regime != RegimeNeutral {
		return ScoreObservation{}, fmt.Errorf("invalid score regime %q", observation.Regime)
	}
	for name, value := range map[string]float64{
		"score": observation.Score, "long_score": observation.LongScore,
		"short_score": observation.ShortScore, "confidence": observation.Confidence,
	} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 100 {
			return ScoreObservation{}, fmt.Errorf("%s must be finite and between 0 and 100", name)
		}
	}
	if observation.FeatureTimestamp.IsZero() {
		return ScoreObservation{}, errors.New("feature timestamp is required")
	}
	if observation.FeatureTimestamp.After(now.Add(time.Second)) {
		return ScoreObservation{}, errors.New("feature timestamp is in the future")
	}
	if now.Sub(observation.FeatureTimestamp) > config.MaxFeatureAge {
		return ScoreObservation{}, errors.New("feature timestamp is stale")
	}
	if observation.PromptVersion == "" || observation.SchemaVersion == "" {
		return ScoreObservation{}, errors.New("prompt and schema versions are required")
	}
	if observation.Provider == "" || observation.Model == "" {
		return ScoreObservation{}, errors.New("provider and model are required")
	}
	if len(observation.Reasons) > config.MaxReasons {
		return ScoreObservation{}, fmt.Errorf("too many score reasons: %d", len(observation.Reasons))
	}
	for _, reason := range observation.Reasons {
		if strings.TrimSpace(reason) == "" || len([]byte(reason)) > config.MaxReasonBytes {
			return ScoreObservation{}, errors.New("score reason is empty or too long")
		}
	}
	if observation.CapturedAt.IsZero() {
		observation.CapturedAt = now
	}
	return observation, nil
}

func configFrom(configs []ScoreValidationConfig) ScoreValidationConfig {
	if len(configs) == 0 {
		return ScoreValidationConfig{}
	}
	return configs[0]
}
