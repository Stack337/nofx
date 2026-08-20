package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nofx/ai500"

	"github.com/gin-gonic/gin"
)

func TestAI500RoutesRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &Server{router: gin.New()}
	server.setupRoutes()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/ai500/latest?symbol=SOLUSDT", nil)

	server.router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

func TestAI500LatestNormalizesSymbolAndDoesNotLeakCredentials(t *testing.T) {
	store := &fakeAI500ReadStore{observations: []ai500.ScoreObservation{{
		ID: "obs-1", Symbol: "SOLUSDT", Score: 88, Provider: "openrouter", Model: "deepseek",
		FeatureTimestamp: time.Now().UTC(), CapturedAt: time.Now().UTC(), PromptVersion: "p1", SchemaVersion: "s1",
	}}}
	server := &Server{ai500Store: store}
	recorder := callAI500Handler(t, server.handleAI500Latest, "/api/ai500/latest?symbol=%20sol-usdt%20")
	if recorder.Code != http.StatusOK || store.latestSymbol != "SOLUSDT" {
		t.Fatalf("status = %d symbol = %q body = %s", recorder.Code, store.latestSymbol, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "sk-secret") || strings.Contains(recorder.Body.String(), "raw_response") {
		t.Fatalf("credential or raw response leaked: %s", recorder.Body.String())
	}
}

func TestAI500ObservationsRejectsUnboundedLimit(t *testing.T) {
	server := &Server{ai500Store: &fakeAI500ReadStore{}}
	for _, limit := range []string{"0", "101", "invalid"} {
		recorder := callAI500Handler(t, server.handleAI500Observations, "/api/ai500/observations?symbol=SOLUSDT&limit="+limit)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("limit %q status = %d, want 400", limit, recorder.Code)
		}
	}
}

func TestAI500HandlersReturnStableEmptyResults(t *testing.T) {
	server := &Server{ai500Store: &fakeAI500ReadStore{}}
	latest := callAI500Handler(t, server.handleAI500Latest, "/api/ai500/latest?symbol=missing")
	if latest.Code != http.StatusNotFound {
		t.Fatalf("latest status = %d, body = %s", latest.Code, latest.Body.String())
	}
	observations := callAI500Handler(t, server.handleAI500Observations, "/api/ai500/observations?symbol=missing&limit=10")
	if observations.Code != http.StatusOK || observations.Body.String() != "{\"observations\":[]}" {
		t.Fatalf("observations status = %d body = %s", observations.Code, observations.Body.String())
	}
}

func TestAI500EvaluationUsesPersistedLabels(t *testing.T) {
	store := &fakeAI500ReadStore{
		observations: []ai500.ScoreObservation{{ID: "obs-1", Symbol: "SOLUSDT", Score: 90, LongScore: 80, ShortScore: 20}},
		outcomes:     []ai500.OutcomeLabel{{ObservationID: "obs-1", Horizon: "1h", ReturnPct: 2, MaxDrawdownPct: -1}},
	}
	server := &Server{ai500Store: store}
	recorder := callAI500Handler(t, server.handleAI500Evaluation, "/api/ai500/evaluation")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var report ai500.EvaluationReport
	if err := json.Unmarshal(recorder.Body.Bytes(), &report); err != nil || report.Count != 1 || report.PrecisionAtK != 100 {
		t.Fatalf("report = %+v error = %v", report, err)
	}
}

func TestConfigureAI500ShadowIsDisabledByDefaultAndCreatesPrivateStoreWhenEnabled(t *testing.T) {
	server := &Server{}
	if err := server.ConfigureAI500Shadow(false, "", nil, 0); err != nil || server.ai500Store != nil {
		t.Fatalf("disabled configuration store = %T error = %v", server.ai500Store, err)
	}
	path := filepath.Join(t.TempDir(), "private", "observations.jsonl")
	if err := server.ConfigureAI500Shadow(true, path, []string{"SOLUSDT"}, 20*time.Minute); err != nil {
		t.Fatalf("enable AI500: %v", err)
	}
	if server.ai500Store == nil || server.ai500ScoreTTL != 20*time.Minute {
		t.Fatalf("store = %T ttl = %v", server.ai500Store, server.ai500ScoreTTL)
	}
}

func callAI500Handler(t *testing.T, handler gin.HandlerFunc, target string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
	handler(ctx)
	return recorder
}

type fakeAI500ReadStore struct {
	observations []ai500.ScoreObservation
	outcomes     []ai500.OutcomeLabel
	latestSymbol string
}

func (s *fakeAI500ReadStore) Put(context.Context, ai500.ScoreObservation) error    { return nil }
func (s *fakeAI500ReadStore) AddOutcome(context.Context, ai500.OutcomeLabel) error { return nil }
func (s *fakeAI500ReadStore) GetLatest(_ context.Context, symbol string) (ai500.ScoreObservation, error) {
	s.latestSymbol = symbol
	for _, observation := range s.observations {
		if observation.Symbol == symbol {
			return observation, nil
		}
	}
	return ai500.ScoreObservation{}, ai500.ErrObservationNotFound
}
func (s *fakeAI500ReadStore) ListObservations(_ context.Context, symbol string, limit int) ([]ai500.ScoreObservation, error) {
	result := make([]ai500.ScoreObservation, 0, len(s.observations))
	for _, observation := range s.observations {
		if symbol == "" || observation.Symbol == symbol {
			result = append(result, observation)
		}
	}
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
func (s *fakeAI500ReadStore) ListOutcomes(context.Context, int) ([]ai500.OutcomeLabel, error) {
	return append([]ai500.OutcomeLabel(nil), s.outcomes...), nil
}
