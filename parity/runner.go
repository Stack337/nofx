package parity

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	paritydomain "nofx/parity/domain"
	"nofx/store"

	"github.com/google/uuid"
)

type AnalysisText string

type AIWorkflow interface {
	Analyze(context.Context, *TradingContextSnapshot) (AnalysisText, error)
	Decide(context.Context, AnalysisText, *TradingContextSnapshot) (DecisionInput, error)
}

type CycleSynchronizer interface {
	Synchronize(context.Context, ExecutionResult) error
}

type NoopSynchronizer struct{}

func (NoopSynchronizer) Synchronize(context.Context, ExecutionResult) error { return nil }

type CyclePersistence interface {
	Create(*paritydomain.Cycle, string, map[string]any) error
	Transition(string, paritydomain.CycleEvent) (*store.ParityCycleRecord, error)
	Complete(string) (*store.ParityCycleRecord, error)
	Fail(string, string, string) (*store.ParityCycleRecord, error)
}

type RunnerConfig struct {
	AgentID    string
	OwnerID    string
	Shadow     bool
	Deadline   time.Duration
	RiskConfig RiskConfig
}

type CycleResult struct {
	CycleID      string
	State        paritydomain.CycleState
	Decision     DecisionInput
	Risk         RiskResult
	Execution    ExecutionResult
	ErrorCode    string
	ErrorMessage string
}

type Runner struct {
	config      RunnerConfig
	persistence CyclePersistence
	context     ContextProvider
	ai          AIWorkflow
	execution   ExecutionPort
	syncer      CycleSynchronizer
	mu          sync.Mutex
	idGenerator func() string
}

func NewRunner(config RunnerConfig, persistence CyclePersistence, contextProvider ContextProvider, ai AIWorkflow, execution ExecutionPort, syncer CycleSynchronizer) *Runner {
	return &Runner{
		config: config, persistence: persistence, context: contextProvider,
		ai: ai, execution: execution, syncer: syncer,
		idGenerator: func() string { return uuid.NewString() },
	}
}

// StartCycle persists a shadow cycle before returning and runs the bounded
// workflow asynchronously. Callers can use the returned ID to poll the API.
func (r *Runner) StartCycle(parent context.Context) (string, error) {
	r.mu.Lock()
	cycleID := r.idGenerator()
	correlationID := uuid.NewString()
	cycle := paritydomain.NewCycle(cycleID, r.config.AgentID, r.config.Shadow)
	cycle.OwnerID = r.config.OwnerID
	if err := r.persistence.Create(cycle, correlationID, map[string]any{"shadow": r.config.Shadow}); err != nil {
		r.mu.Unlock()
		return "", fmt.Errorf("create cycle: %w", err)
	}
	r.mu.Unlock()
	go func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		_, _ = r.runCycle(context.WithoutCancel(parent), cycleID, false)
	}()
	return cycleID, nil
}

// RunCycle serializes one agent's work and persists every boundary before
// invoking the next component. It always returns a terminal state.
func (r *Runner) RunCycle(parent context.Context) (CycleResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	cycleID := r.idGenerator()
	return r.runCycle(parent, cycleID, true)
}

func (r *Runner) runCycle(parent context.Context, cycleID string, create bool) (CycleResult, error) {
	correlationID := uuid.NewString()
	cycle := paritydomain.NewCycle(cycleID, r.config.AgentID, r.config.Shadow)
	cycle.OwnerID = r.config.OwnerID
	result := CycleResult{CycleID: cycleID, State: paritydomain.CycleScheduled}
	if create {
		if err := r.persistence.Create(cycle, correlationID, map[string]any{"shadow": r.config.Shadow}); err != nil {
			return result, fmt.Errorf("create cycle: %w", err)
		}
	}

	deadline := r.config.Deadline
	if deadline <= 0 {
		deadline = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(parent, deadline)
	defer cancel()

	if _, err := r.persistence.Transition(cycleID, paritydomain.EventCollectContext); err != nil {
		return r.fail(cycleID, "state_transition_failed", err)
	}
	snapshot, err := BuildTradingContext(ctx, r.context, time.Now().UTC())
	if err != nil {
		return r.fail(cycleID, stageErrorCode(ctx, "context_failed"), err)
	}

	if _, err := r.persistence.Transition(cycleID, paritydomain.EventStartAnalysis); err != nil {
		return r.fail(cycleID, "state_transition_failed", err)
	}
	analysis, err := r.ai.Analyze(ctx, snapshot)
	if err != nil {
		return r.fail(cycleID, stageErrorCode(ctx, "analysis_failed"), err)
	}

	if _, err := r.persistence.Transition(cycleID, paritydomain.EventStartToolRound); err != nil {
		return r.fail(cycleID, "state_transition_failed", err)
	}
	decision, err := r.ai.Decide(ctx, analysis, snapshot)
	if err != nil {
		code := stageErrorCode(ctx, "decision_failed")
		var protocolErr *ProtocolError
		if errors.As(err, &protocolErr) && protocolErr.Code != "" {
			code = protocolErr.Code
		}
		return r.fail(cycleID, code, err)
	}
	result.Decision = decision

	if _, err := r.persistence.Transition(cycleID, paritydomain.EventStartValidation); err != nil {
		return r.fail(cycleID, "state_transition_failed", err)
	}
	risk := ValidateDecision(decision, snapshot.Account.TotalEquity, snapshot.CollectedAt, r.config.RiskConfig)
	result.Risk = risk
	if !risk.Allowed {
		return r.fail(cycleID, risk.ReasonCode, fmt.Errorf("risk rejected action: %s", risk.ReasonCode))
	}
	result.Decision = risk.Decision

	if _, err := r.persistence.Transition(cycleID, paritydomain.EventStartExecution); err != nil {
		return r.fail(cycleID, "state_transition_failed", err)
	}
	execution, err := r.execution.Execute(ctx, risk.Decision)
	if err != nil {
		return r.fail(cycleID, stageErrorCode(ctx, "execution_failed"), err)
	}
	result.Execution = execution

	if _, err := r.persistence.Transition(cycleID, paritydomain.EventStartSynchronization); err != nil {
		return r.fail(cycleID, "state_transition_failed", err)
	}
	if err := r.syncer.Synchronize(ctx, execution); err != nil {
		return r.fail(cycleID, stageErrorCode(ctx, "synchronization_failed"), err)
	}
	if _, err := r.persistence.Complete(cycleID); err != nil {
		return r.fail(cycleID, "state_transition_failed", err)
	}
	result.State = paritydomain.CycleCompleted
	return result, nil
}

func (r *Runner) fail(cycleID, code string, cause error) (CycleResult, error) {
	message := cause.Error()
	record, persistErr := r.persistence.Fail(cycleID, code, message)
	result := CycleResult{
		CycleID: cycleID, State: paritydomain.CycleFailed,
		ErrorCode: code, ErrorMessage: message,
	}
	if persistErr != nil {
		return result, errors.Join(cause, fmt.Errorf("persist terminal cycle failure: %w", persistErr))
	}
	result.State = record.State
	return result, cause
}

func stageErrorCode(ctx context.Context, fallback string) string {
	if ctx.Err() != nil {
		return "cycle_timeout"
	}
	return fallback
}
