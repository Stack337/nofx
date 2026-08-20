package ai500

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	PromptVersionV1 = "ai500-v1"
	SchemaVersionV1 = "ai500-score-v1"
)

type ScoreRequest struct {
	Symbol        string          `json:"symbol"`
	Features      FeatureSnapshot `json:"features"`
	PromptVersion string          `json:"prompt_version"`
	SchemaVersion string          `json:"schema_version"`
}

type ScoreResponse struct {
	RawJSON  []byte
	Provider string
	Model    string
}

type ScoreModel interface {
	Score(context.Context, ScoreRequest) (ScoreResponse, error)
}

type ScoreError struct {
	Code        string
	Message     string
	Cause       error
	RawResponse []byte
}

func (e *ScoreError) Error() string { return e.Message }
func (e *ScoreError) Unwrap() error { return e.Cause }

type Workflow struct {
	model      ScoreModel
	validation ScoreValidationConfig
}

func NewWorkflow(model ScoreModel, validation ScoreValidationConfig) *Workflow {
	return &Workflow{model: model, validation: validation}
}

func (w *Workflow) Score(ctx context.Context, features FeatureSnapshot) (ScoreObservation, error) {
	if w == nil || w.model == nil {
		return ScoreObservation{}, scoreError("score_unavailable", "AI500 score model is unavailable", nil)
	}
	if !features.DataComplete || strings.TrimSpace(features.Symbol) == "" || features.SourceTimestamp.IsZero() {
		return ScoreObservation{}, scoreError("score_unavailable", "AI500 features are incomplete", nil)
	}
	request := ScoreRequest{
		Symbol: features.Symbol, Features: features,
		PromptVersion: PromptVersionV1, SchemaVersion: SchemaVersionV1,
	}
	response, err := w.model.Score(ctx, request)
	if err != nil {
		return ScoreObservation{}, scoreError("score_upstream_error", "AI500 score provider failed", err)
	}
	parsed, err := parseScoreResponse(response.RawJSON)
	if err != nil {
		return ScoreObservation{}, scoreError("score_invalid", "AI500 provider returned an invalid score", err)
	}
	now := w.validation.Now
	capturedAt := features.BuiltAt
	if now != nil {
		capturedAt = now()
	}
	observation := ScoreObservation{
		Symbol: features.Symbol, Score: *parsed.Score,
		LongScore: *parsed.LongScore, ShortScore: *parsed.ShortScore,
		Confidence: *parsed.Confidence, Regime: *parsed.Regime,
		Reasons:          append([]string(nil), parsed.Reasons...),
		FeatureTimestamp: features.SourceTimestamp, CapturedAt: capturedAt,
		Provider: response.Provider, Model: response.Model,
		PromptVersion: request.PromptVersion, SchemaVersion: request.SchemaVersion,
	}
	normalized, err := NormalizeScore(observation, w.validation)
	if err != nil {
		return ScoreObservation{}, scoreError("score_invalid", "AI500 score failed validation", err)
	}
	return normalized, nil
}

type scoreResponseDTO struct {
	Score      *float64 `json:"score"`
	LongScore  *float64 `json:"long_score"`
	ShortScore *float64 `json:"short_score"`
	Confidence *float64 `json:"confidence"`
	Regime     *Regime  `json:"regime"`
	Reasons    []string `json:"reasons"`
}

func parseScoreResponse(raw []byte) (scoreResponseDTO, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var parsed scoreResponseDTO
	if err := decoder.Decode(&parsed); err != nil {
		return scoreResponseDTO{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return scoreResponseDTO{}, errors.New("score response contains trailing JSON")
	}
	if parsed.Score == nil || parsed.LongScore == nil || parsed.ShortScore == nil || parsed.Confidence == nil || parsed.Regime == nil || parsed.Reasons == nil {
		return scoreResponseDTO{}, errors.New("score response is missing required fields")
	}
	return parsed, nil
}

func scoreError(code, message string, cause error) *ScoreError {
	if cause != nil && errors.Is(cause, context.Canceled) {
		message = fmt.Sprintf("%s: request canceled", message)
	}
	return &ScoreError{Code: code, Message: message, Cause: cause}
}
