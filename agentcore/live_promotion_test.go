package agentcore

import (
	"context"
	"errors"
	"testing"

	"nofx/agent"
	"nofx/execution"
	"nofx/provider"
	"nofx/risk"
)

type rejectingHealth struct{ err error }

func (h rejectingHealth) Ready(context.Context, agent.Agent) error { return h.err }

type passingHealth struct{}

func (passingHealth) Ready(context.Context, agent.Agent) error { return nil }

func liveService(cycles *fakeCycleRepo) *Service {
	service := minimalService(cycles)
	service.Agents = &fakeAgentRepo{value: agent.Agent{ID: "agent-1", Mode: agent.ModeLive, Enabled: true, LiveConfirmed: true}}
	return service
}

func TestLiveCycleFailsClosedWithoutHealthGate(t *testing.T) {
	cycles := &fakeCycleRepo{}
	service := liveService(cycles)
	cycle, err := service.RunCycle(context.Background(), "agent-1")
	if err == nil || cycle.Status != agent.CycleFailed || cycle.ErrorCode != "live_health_gate_missing" {
		t.Fatalf("RunCycle() = %+v, %v", cycle, err)
	}
}

func TestLiveCycleRequiresPassingHealthGate(t *testing.T) {
	cycles := &fakeCycleRepo{}
	service := liveService(cycles)
	service.Health = rejectingHealth{err: errors.New("exchange unavailable")}
	cycle, err := service.RunCycle(context.Background(), "agent-1")
	if err == nil || cycle.Status != agent.CycleFailed || cycle.ErrorCode != "live_preflight_failed" {
		t.Fatalf("RunCycle() = %+v, %v", cycle, err)
	}
}

func TestLiveCycleRequiresExplicitConfirmation(t *testing.T) {
	cycles := &fakeCycleRepo{}
	service := liveService(cycles)
	service.Health = passingHealth{}
	service.Agents = &fakeAgentRepo{value: agent.Agent{ID: "agent-1", Mode: agent.ModeLive, Enabled: true, LiveConfirmed: false}}
	cycle, err := service.RunCycle(context.Background(), "agent-1")
	if err == nil || cycle.Status != agent.CycleFailed || cycle.ErrorCode != "live_unconfirmed" {
		t.Fatalf("RunCycle() = %+v, %v", cycle, err)
	}
}

func TestLivePromotionMapsProviderDecisionErrors(t *testing.T) {
	cycles := &fakeCycleRepo{}
	service := liveService(cycles)
	service.Health = passingHealth{}
	service.Provider = fakeProvider{err: &provider.DecisionError{Code: provider.DecisionErrorEmptyResponse, Message: "empty"}}
	cycle, err := service.RunCycle(context.Background(), "agent-1")
	if err == nil || cycle.Status != agent.CycleFailed || cycle.ErrorCode != provider.DecisionErrorEmptyResponse {
		t.Fatalf("RunCycle() = %+v, %v", cycle, err)
	}
}

func TestLivePromotionMapsRiskRejectionCodes(t *testing.T) {
	cycles := &fakeCycleRepo{}
	service := liveService(cycles)
	service.Health = passingHealth{}
	service.Risk = fakeRisk{err: &risk.Rejection{Code: risk.CodeProtectionRequired, Message: "sl/tp required"}}
	cycle, err := service.RunCycle(context.Background(), "agent-1")
	if err == nil || cycle.Status != agent.CycleFailed || cycle.ErrorCode != risk.CodeProtectionRequired {
		t.Fatalf("RunCycle() = %+v, %v", cycle, err)
	}
}

func TestLivePromotionMapsExchangeFailureCodes(t *testing.T) {
	cycles := &fakeCycleRepo{}
	service := liveService(cycles)
	service.Health = passingHealth{}
	service.Router = &fakeRouter{err: execution.HTTPStatusError(429, "rate limited")}
	cycle, err := service.RunCycle(context.Background(), "agent-1")
	if err == nil || cycle.Status != agent.CycleFailed || cycle.ErrorCode != "exchange_http_429" {
		t.Fatalf("RunCycle() = %+v, %v", cycle, err)
	}
}

func TestShadowAndPaperCyclesStayTerminalWithoutLiveGate(t *testing.T) {
	for _, mode := range []agent.Mode{agent.ModeShadow, agent.ModePaper} {
		cycles := &fakeCycleRepo{}
		service := minimalService(cycles)
		service.Agents = &fakeAgentRepo{value: agent.Agent{ID: "agent-1", Mode: mode, Enabled: true}}
		cycle, err := service.RunCycle(context.Background(), "agent-1")
		if err != nil || cycle.Status != agent.CycleCompleted {
			t.Fatalf("mode %s RunCycle() = %+v, %v", mode, cycle, err)
		}
	}
}
