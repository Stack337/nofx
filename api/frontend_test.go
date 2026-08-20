package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestConfigureFrontendServesIndexAndSPARoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dist := t.TempDir()
	if err := os.Mkdir(filepath.Join(dist, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("private-agent-ui"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "assets", "app.js"), []byte("app-js"), 0o644); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	configureFrontend(router, dist)
	for _, target := range []string{"/", "/agents", "/assets/app.js"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d", target, recorder.Code)
		}
	}
}

func TestConfigureFrontendKeepsUnknownAPIRoutesAs404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dist := t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("private-agent-ui"), 0o644); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	configureFrontend(router, dist)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/missing", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}
