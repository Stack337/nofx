package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"nofx/ai500"
	"nofx/market"

	"github.com/gin-gonic/gin"
)

type ai500ReadStore interface {
	ListObservations(context.Context, string, int) ([]ai500.ScoreObservation, error)
	ListOutcomes(context.Context, int) ([]ai500.OutcomeLabel, error)
}

type ai500ObservationResponse struct {
	ID               string       `json:"id"`
	Symbol           string       `json:"symbol"`
	Score            float64      `json:"score"`
	LongScore        float64      `json:"long_score"`
	ShortScore       float64      `json:"short_score"`
	Confidence       float64      `json:"confidence"`
	Regime           ai500.Regime `json:"regime"`
	Reasons          []string     `json:"reasons,omitempty"`
	FeatureTimestamp string       `json:"feature_timestamp"`
	CapturedAt       string       `json:"captured_at"`
	Provider         string       `json:"provider"`
	Model            string       `json:"model"`
	PromptVersion    string       `json:"prompt_version"`
	SchemaVersion    string       `json:"schema_version"`
}

func (s *Server) handleAI500Latest(c *gin.Context) {
	if s.ai500Store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI500 observation store is unavailable", "error_code": "ai500_store_unavailable"})
		return
	}
	symbol := normalizeAI500Symbol(c.Query("symbol"))
	if symbol == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "symbol is required"})
		return
	}
	observation, err := s.ai500Store.GetLatest(c.Request.Context(), symbol)
	if errors.Is(err, ai500.ErrObservationNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "AI500 observation not found", "error_code": "ai500_observation_not_found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load AI500 observation"})
		return
	}
	c.JSON(http.StatusOK, ai500ObservationResponseFrom(observation))
}

func (s *Server) handleAI500Observations(c *gin.Context) {
	store, ok := s.ai500Store.(ai500ReadStore)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI500 observation read model is unavailable", "error_code": "ai500_read_model_unavailable"})
		return
	}
	limit, err := boundedAI500Limit(c.Query("limit"), 20)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	symbol := normalizeAI500Symbol(c.Query("symbol"))
	observations, err := store.ListObservations(c.Request.Context(), symbol, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load AI500 observations"})
		return
	}
	result := make([]ai500ObservationResponse, 0, len(observations))
	for _, observation := range observations {
		result = append(result, ai500ObservationResponseFrom(observation))
	}
	c.JSON(http.StatusOK, gin.H{"observations": result})
}

func (s *Server) handleAI500Evaluation(c *gin.Context) {
	store, ok := s.ai500Store.(ai500ReadStore)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI500 evaluation read model is unavailable", "error_code": "ai500_read_model_unavailable"})
		return
	}
	observations, err := store.ListObservations(c.Request.Context(), "", 10000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load AI500 observations"})
		return
	}
	outcomes, err := store.ListOutcomes(c.Request.Context(), 10000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load AI500 outcomes"})
		return
	}
	c.JSON(http.StatusOK, ai500.Evaluate(observations, outcomes, 10))
}

func boundedAI500Limit(raw string, defaultLimit int) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return defaultLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > 100 {
		return 0, errors.New("limit must be between 1 and 100")
	}
	return limit, nil
}

func normalizeAI500Symbol(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return market.Normalize(raw)
}

func ai500ObservationResponseFrom(observation ai500.ScoreObservation) ai500ObservationResponse {
	return ai500ObservationResponse{ID: observation.ID, Symbol: observation.Symbol, Score: observation.Score, LongScore: observation.LongScore, ShortScore: observation.ShortScore, Confidence: observation.Confidence, Regime: observation.Regime, Reasons: append([]string(nil), observation.Reasons...), FeatureTimestamp: observation.FeatureTimestamp.UTC().Format("2006-01-02T15:04:05.000Z07:00"), CapturedAt: observation.CapturedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"), Provider: observation.Provider, Model: observation.Model, PromptVersion: observation.PromptVersion, SchemaVersion: observation.SchemaVersion}
}
