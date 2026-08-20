package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"nofx/agent"
	"nofx/store"
)

func openAgentAPIStore(t *testing.T) *store.Store {
	t.Helper()
	gdb, err := gorm.Open(sqlite.New(sqlite.Config{DriverName: "sqlite", DSN: filepath.Join(t.TempDir(), "agent-api.db")}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	s, err := store.NewFromGorm(gdb)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := s.Agent().InitTables(); err != nil {
		t.Fatalf("init tables: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func seedAgent(t *testing.T, s *store.Store, id, userID string) agent.Agent {
	t.Helper()
	item := agent.Agent{ID: id, UserID: userID, Name: "flash", ExchangeID: "bybit-1", AIModelID: "deepseek", Mode: agent.ModeShadow, Enabled: true}
	if err := s.Agent().Create(t.Context(), item); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return item
}

func callAgentHandler(t *testing.T, handler gin.HandlerFunc, method, target, userID, body string, params gin.Params) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = params
	ctx.Set("user_id", userID)
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	ctx.Request = req
	handler(ctx)
	return recorder
}

func TestLiveConfirmationRequiresExactPhrase(t *testing.T) {
	s := openAgentAPIStore(t)
	item := seedAgent(t, s, "agent-1", "user-1")
	server := &Server{store: s}
	recorder := callAgentHandler(t, server.handleAgentLiveConfirmation, http.MethodPost, "/api/agents/agent-1/live-confirmation", "user-1", `{"confirmation":"enable live"}`, gin.Params{{Key: "id", Value: item.ID}})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	got, err := s.Agent().Get(t.Context(), item.ID)
	if err != nil || got.Mode != agent.ModeShadow || got.LiveConfirmed {
		t.Fatalf("agent = %+v err = %v", got, err)
	}
}

func TestLiveConfirmationEnablesExactlyOneAgent(t *testing.T) {
	s := openAgentAPIStore(t)
	item := seedAgent(t, s, "agent-1", "user-1")
	other := seedAgent(t, s, "agent-2", "user-1")
	server := &Server{store: s}
	recorder := callAgentHandler(t, server.handleAgentLiveConfirmation, http.MethodPost, "/api/agents/agent-1/live-confirmation", "user-1", `{"confirmation":"ENABLE LIVE agent-1"}`, gin.Params{{Key: "id", Value: item.ID}})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload["mode"] != string(agent.ModeLive) || payload["live_confirmed"] != true {
		t.Fatalf("payload = %#v", payload)
	}
	first, _ := s.Agent().Get(t.Context(), item.ID)
	second, _ := s.Agent().Get(t.Context(), other.ID)
	if first.Mode != agent.ModeLive || !first.LiveConfirmed || second.Mode != agent.ModeShadow || second.LiveConfirmed {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
}

func TestAgentStartDoesNotEnableLive(t *testing.T) {
	s := openAgentAPIStore(t)
	item := seedAgent(t, s, "agent-1", "user-1")
	server := &Server{store: s}
	recorder := callAgentHandler(t, server.handleAgentStart, http.MethodPost, "/api/agents/agent-1/start", "user-1", "", gin.Params{{Key: "id", Value: item.ID}})
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body = %s, want 503 without agent service", recorder.Code, recorder.Body.String())
	}
	got, err := s.Agent().Get(t.Context(), item.ID)
	if err != nil || got.Mode != agent.ModeShadow || got.LiveConfirmed {
		t.Fatalf("agent = %+v err = %v", got, err)
	}
}

func TestKillSwitchDisableRequiresToken(t *testing.T) {
	s := openAgentAPIStore(t)
	item := seedAgent(t, s, "agent-1", "user-1")
	server := &Server{store: s}
	enable := callAgentHandler(t, server.handleAgentKillSwitch, http.MethodPost, "/api/agents/agent-1/kill-switch", "user-1", `{"enabled":true,"reason":"halt"}`, gin.Params{{Key: "id", Value: item.ID}})
	if enable.Code != http.StatusOK {
		t.Fatalf("enable status = %d body = %s", enable.Code, enable.Body.String())
	}
	wrong := callAgentHandler(t, server.handleAgentKillSwitch, http.MethodPost, "/api/agents/agent-1/kill-switch", "user-1", `{"enabled":false,"confirmation":"yes"}`, gin.Params{{Key: "id", Value: item.ID}})
	if wrong.Code != http.StatusBadRequest {
		t.Fatalf("wrong disable status = %d body = %s", wrong.Code, wrong.Body.String())
	}
	ok := callAgentHandler(t, server.handleAgentKillSwitch, http.MethodPost, "/api/agents/agent-1/kill-switch", "user-1", `{"enabled":false,"confirmation":"DISABLE agent-1"}`, gin.Params{{Key: "id", Value: item.ID}})
	if ok.Code != http.StatusOK {
		t.Fatalf("confirmed disable status = %d body = %s", ok.Code, ok.Body.String())
	}
}

func TestAgentLiveRoutesRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &Server{router: gin.New()}
	server.setupRoutes()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agents/agent-1/live-confirmation", strings.NewReader(`{"confirmation":"ENABLE LIVE agent-1"}`))
	req.Header.Set("Content-Type", "application/json")
	server.router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}
