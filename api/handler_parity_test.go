package api

import (
	"net/http/httptest"
	"nofx/parity"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHandleParityCyclesRequiresAgentID(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	server := &Server{}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("GET", "/api/parity/cycles", nil)

	server.handleParityCycles(context)
	if recorder.Code != 400 {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestHandleParityCyclesRejectsInvalidLimitBeforeStoreAccess(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	server := &Server{}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("GET", "/api/parity/cycles?agent_id=agent-1&limit=101", nil)

	server.handleParityCycles(context)
	if recorder.Code != 400 {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestHandleStartParityCycleRejectsUnregisteredRunner(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	server := &Server{parityRunners: map[string]*parity.Runner{}}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("POST", "/api/parity/cycles/run?agent_id=agent-1", nil)

	server.handleStartParityCycle(context)
	if recorder.Code != 503 {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func TestHandleRegisterParityRunnerRequiresAgentID(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	server := &Server{}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("POST", "/api/parity/runners", nil)

	server.handleRegisterParityRunner(context)
	if recorder.Code != 400 {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestHandleRegisterParityRunnerRejectsMissingAI500Store(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	server := &Server{}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("POST", "/api/parity/runners?agent_id=agent-1", nil)

	server.handleRegisterParityRunner(context)
	if recorder.Code != 503 {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}
