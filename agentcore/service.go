package agentcore

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"nofx/agent"
	"nofx/ai500"
	"nofx/execution"
	"nofx/provider"
	"nofx/risk"
)

type AgentRepository interface {
	Get(context.Context, string) (agent.Agent, error)
}
type CycleRepository interface {
	CreateCycle(context.Context, agent.AgentCycle) error
	TransitionCycle(context.Context, string, agent.CycleStatus, agent.CycleStatus, string) error
}
type ContextCollector interface {
	Collect(context.Context, agent.Agent) ([]ai500.ScoreObservation, risk.AccountRisk, error)
}
type Ranker interface {
	Rank(context.Context, []ai500.ScoreObservation, ai500.RankConfig) ([]ai500.Candidate, error)
}
type Provider interface {
	Decide(context.Context, provider.DecisionRequest) (provider.DecisionResponse, provider.ProviderMeta, error)
}
type RiskEngine interface {
	Authorize(context.Context, provider.DecisionResponse, risk.AccountRisk, risk.Policy) (risk.AuthorizedDecision, error)
}
type ExecutionRouter interface {
	Execute(context.Context, agent.Mode, risk.AuthorizedDecision, string) (execution.OrderResult, error)
}

type AuditEvent struct {
	Type, AgentID, CycleID, ErrorCode string
	At                                time.Time
}
type AuditWriter interface {
	Append(context.Context, AuditEvent) error
}

type Service struct {
	Agents       AgentRepository
	Cycles       CycleRepository
	Collector    ContextCollector
	Ranker       Ranker
	Provider     Provider
	Risk         RiskEngine
	Router       ExecutionRouter
	Audit        AuditWriter
	Policy       risk.Policy
	RankConfig   ai500.RankConfig
	Now          func() time.Time
	CycleTimeout time.Duration
	mu           sync.Mutex
	running      map[string]struct{}
}

func (s *Service) RunCycle(parent context.Context, agentID string) (agent.AgentCycle, error) {
	if !s.acquire(agentID) {
		return agent.AgentCycle{}, fmt.Errorf("agent %s cycle already running", agentID)
	}
	defer s.release(agentID)
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	timeout := s.CycleTimeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	configuration, err := s.Agents.Get(ctx, agentID)
	if err != nil {
		return agent.AgentCycle{}, err
	}
	if !configuration.Enabled {
		return agent.AgentCycle{}, fmt.Errorf("agent %s is disabled", agentID)
	}
	cycle := agent.AgentCycle{ID: fmt.Sprintf("%s-%d", agentID, now.UnixNano()), AgentID: agentID, Status: agent.CycleQueued, Deadline: now.Add(timeout)}
	if err := s.Cycles.CreateCycle(ctx, cycle); err != nil {
		return cycle, err
	}
	if err := s.Cycles.TransitionCycle(ctx, cycle.ID, agent.CycleQueued, agent.CycleProcessing, ""); err != nil {
		return cycle, err
	}
	cycle.Status = agent.CycleProcessing
	fail := func(status agent.CycleStatus, code string, cause error) (agent.AgentCycle, error) {
		_ = s.Cycles.TransitionCycle(context.WithoutCancel(parent), cycle.ID, agent.CycleProcessing, status, code)
		cycle.Status, cycle.ErrorCode = status, code
		if s.Audit != nil {
			_ = s.Audit.Append(context.WithoutCancel(parent), AuditEvent{Type: "cycle." + string(status), AgentID: agentID, CycleID: cycle.ID, ErrorCode: code, At: time.Now().UTC()})
		}
		return cycle, cause
	}
	observations, accountRisk, err := s.Collector.Collect(ctx, configuration)
	if err != nil {
		return fail(agent.CycleFailed, "market_context_failed", err)
	}
	candidates, err := s.Ranker.Rank(ctx, observations, s.RankConfig)
	if err != nil {
		return fail(agent.CycleFailed, "ranking_failed", err)
	}
	views := make([]provider.CandidateView, 0, len(candidates))
	symbols := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		views = append(views, provider.CandidateView{Symbol: candidate.Symbol, Score: candidate.Score, LongScore: candidate.LongScore, ShortScore: candidate.ShortScore, Confidence: candidate.Confidence, Regime: string(candidate.Regime), Reasons: append([]string(nil), candidate.Reasons...)})
		symbols = append(symbols, candidate.Symbol)
	}
	decision, _, err := s.Provider.Decide(ctx, provider.DecisionRequest{CycleID: cycle.ID, AgentID: agentID, Candidates: views, AllowedActions: []agent.DecisionAction{agent.DecisionOpen, agent.DecisionClose, agent.DecisionHold}})
	if err != nil {
		return fail(agent.CycleFailed, "provider_failed", err)
	}
	if decision.GeneratedAt.IsZero() {
		decision.GeneratedAt = now
	}
	authorized, err := s.Risk.Authorize(ctx, decision, accountRisk, s.Policy)
	if err != nil {
		return fail(agent.CycleFailed, "risk_rejected", err)
	}
	authorized.LiveConfirmed = configuration.LiveConfirmed
	_, err = s.Router.Execute(ctx, configuration.Mode, authorized, cycle.ID)
	if errors.Is(err, execution.ErrNeedsReconciliation) {
		return fail(agent.CycleNeedsReconciliation, "order_unknown", err)
	}
	if err != nil {
		return fail(agent.CycleFailed, "execution_failed", err)
	}
	if err := s.Cycles.TransitionCycle(ctx, cycle.ID, agent.CycleProcessing, agent.CycleCompleted, ""); err != nil {
		return cycle, err
	}
	cycle.Status = agent.CycleCompleted
	if s.Audit != nil {
		_ = s.Audit.Append(ctx, AuditEvent{Type: "cycle.completed", AgentID: agentID, CycleID: cycle.ID, At: time.Now().UTC()})
	}
	return cycle, nil
}

func (s *Service) acquire(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running == nil {
		s.running = map[string]struct{}{}
	}
	if _, ok := s.running[id]; ok {
		return false
	}
	s.running[id] = struct{}{}
	return true
}
func (s *Service) release(id string) { s.mu.Lock(); delete(s.running, id); s.mu.Unlock() }
