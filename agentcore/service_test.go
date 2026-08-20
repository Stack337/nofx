package agentcore

import (
	"context"
	"errors"
	"testing"
	"time"

	"nofx/agent"
	"nofx/ai500"
	"nofx/execution"
	"nofx/provider"
	"nofx/risk"
)

type fakeAgentRepo struct{ value agent.Agent }

func (r *fakeAgentRepo) Get(context.Context, string) (agent.Agent, error) { return r.value, nil }

type fakeCycleRepo struct {
	created     agent.AgentCycle
	transitions []agent.CycleStatus
}

func (r *fakeCycleRepo) CreateCycle(_ context.Context, cycle agent.AgentCycle) error {
	r.created = cycle
	return nil
}
func (r *fakeCycleRepo) TransitionCycle(_ context.Context, _ string, _, to agent.CycleStatus, _ string) error {
	r.transitions = append(r.transitions, to)
	return nil
}

type fakeCollector struct {
	observations []ai500.ScoreObservation
	risk         risk.AccountRisk
	err          error
}

func (c fakeCollector) Collect(context.Context, agent.Agent) ([]ai500.ScoreObservation, risk.AccountRisk, error) {
	return c.observations, c.risk, c.err
}

type fakeRanker struct{ candidates []ai500.Candidate }

func (r fakeRanker) Rank(context.Context, []ai500.ScoreObservation, ai500.RankConfig) ([]ai500.Candidate, error) {
	return r.candidates, nil
}

type fakeProvider struct {
	decision provider.DecisionResponse
	err      error
}

func (p fakeProvider) Decide(context.Context, provider.DecisionRequest) (provider.DecisionResponse, provider.ProviderMeta, error) {
	return p.decision, provider.ProviderMeta{}, p.err
}

type fakeRisk struct {
	authorized risk.AuthorizedDecision
	err        error
}

func (r fakeRisk) Authorize(context.Context, provider.DecisionResponse, risk.AccountRisk, risk.Policy) (risk.AuthorizedDecision, error) {
	return r.authorized, r.err
}

type fakeRouter struct {
	result execution.OrderResult
	err    error
	calls  int
}

func (r *fakeRouter) Execute(context.Context, agent.Mode, risk.AuthorizedDecision, string) (execution.OrderResult, error) {
	r.calls++
	return r.result, r.err
}

type fakeAudit struct{ events int }

func (a *fakeAudit) Append(context.Context, AuditEvent) error { a.events++; return nil }

func TestServiceCompletesHoldCycle(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	cycles := &fakeCycleRepo{}
	router := &fakeRouter{result: execution.OrderResult{Status: execution.OrderStatusPaper}}
	service := &Service{
		Agents: &fakeAgentRepo{value: agent.Agent{ID: "agent-1", Mode: agent.ModePaper, Enabled: true}},
		Cycles: cycles, Collector: fakeCollector{}, Ranker: fakeRanker{},
		Provider: fakeProvider{decision: provider.DecisionResponse{Action: agent.DecisionHold, GeneratedAt: now}},
		Risk:     fakeRisk{authorized: risk.AuthorizedDecision{Decision: provider.DecisionResponse{Action: agent.DecisionHold}}},
		Router:   router, Audit: &fakeAudit{}, Now: func() time.Time { return now }, CycleTimeout: time.Second,
	}
	cycle, err := service.RunCycle(context.Background(), "agent-1")
	if err != nil || cycle.Status != agent.CycleCompleted || router.calls != 1 {
		t.Fatalf("RunCycle() = %+v, %v, router calls %d", cycle, err, router.calls)
	}
}

func TestServiceMapsUnknownOrderToNeedsReconciliation(t *testing.T) {
	cycles := &fakeCycleRepo{}
	service := minimalService(cycles)
	service.Router = &fakeRouter{err: execution.ErrNeedsReconciliation}
	cycle, err := service.RunCycle(context.Background(), "agent-1")
	if !errors.Is(err, execution.ErrNeedsReconciliation) || cycle.Status != agent.CycleNeedsReconciliation {
		t.Fatalf("RunCycle() = %+v, %v", cycle, err)
	}
}

func TestServiceProviderFailureTerminatesCycle(t *testing.T) {
	cycles := &fakeCycleRepo{}
	service := minimalService(cycles)
	service.Provider = fakeProvider{err: errors.New("provider down")}
	cycle, err := service.RunCycle(context.Background(), "agent-1")
	if err == nil || cycle.Status != agent.CycleFailed {
		t.Fatalf("RunCycle() = %+v, %v", cycle, err)
	}
}

func minimalService(cycles *fakeCycleRepo) *Service {
	now := time.Now().UTC()
	decision := provider.DecisionResponse{Action: agent.DecisionHold, GeneratedAt: now}
	return &Service{
		Agents: &fakeAgentRepo{value: agent.Agent{ID: "agent-1", Mode: agent.ModeShadow, Enabled: true}},
		Cycles: cycles, Collector: fakeCollector{}, Ranker: fakeRanker{}, Provider: fakeProvider{decision: decision},
		Risk: fakeRisk{authorized: risk.AuthorizedDecision{Decision: decision}}, Router: &fakeRouter{}, Audit: &fakeAudit{},
		Now: func() time.Time { return now }, CycleTimeout: time.Second,
	}
}
