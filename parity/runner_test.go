package parity

import (
	"context"
	"errors"
	"testing"
	"time"

	paritydomain "nofx/parity/domain"
	"nofx/store"
)

func TestRunnerCompletesShadowCycle(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	persistence := newFakeCyclePersistence()
	runner := NewRunner(RunnerConfig{
		AgentID:    "agent-1",
		Shadow:     true,
		Deadline:   time.Second,
		RiskConfig: defaultRiskConfig(),
	}, persistence, runnerContextProvider(now), fakeAIWorkflow{decision: validLong(now)}, NewShadowExecution(), noopSynchronizer{})
	runner.idGenerator = func() string { return "cycle-success" }

	result, err := runner.RunCycle(context.Background())
	if err != nil {
		t.Fatalf("run cycle: %v", err)
	}
	if result.State != paritydomain.CycleCompleted || !result.Execution.Simulated {
		t.Fatalf("result = %+v", result)
	}
	if persistence.state("cycle-success") != paritydomain.CycleCompleted {
		t.Fatalf("persisted state = %q", persistence.state("cycle-success"))
	}
}

func TestRunnerPersistsTerminalFailureForEveryStage(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	tests := []struct {
		name      string
		ai        AIWorkflow
		execution ExecutionPort
		syncer    CycleSynchronizer
		deadline  time.Duration
		wantCode  string
	}{
		{
			name:      "analysis failure",
			ai:        fakeAIWorkflow{analysisErr: errors.New("provider unavailable")},
			execution: NewShadowExecution(), syncer: noopSynchronizer{}, deadline: time.Second,
			wantCode: "analysis_failed",
		},
		{
			name:      "missing tool call",
			ai:        fakeAIWorkflow{decisionErr: &ProtocolError{Code: "tool_call_missing", Message: "missing"}},
			execution: NewShadowExecution(), syncer: noopSynchronizer{}, deadline: time.Second,
			wantCode: "tool_call_missing",
		},
		{
			name: "risk rejection",
			ai: fakeAIWorkflow{decision: func() DecisionInput {
				decision := validLong(now)
				decision.TakeProfit = 101
				return decision
			}()},
			execution: NewShadowExecution(), syncer: noopSynchronizer{}, deadline: time.Second,
			wantCode: "risk_reward_too_low",
		},
		{
			name: "execution rejection",
			ai:   fakeAIWorkflow{decision: validLong(now)}, execution: failingExecution{}, syncer: noopSynchronizer{}, deadline: time.Second,
			wantCode: "execution_failed",
		},
		{
			name: "synchronization failure",
			ai:   fakeAIWorkflow{decision: validLong(now)}, execution: NewShadowExecution(), syncer: failingSynchronizer{}, deadline: time.Second,
			wantCode: "synchronization_failed",
		},
		{
			name: "watchdog timeout",
			ai:   blockingAIWorkflow{}, execution: NewShadowExecution(), syncer: noopSynchronizer{}, deadline: 20 * time.Millisecond,
			wantCode: "cycle_timeout",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			persistence := newFakeCyclePersistence()
			runner := NewRunner(RunnerConfig{
				AgentID: "agent-1", Shadow: true, Deadline: tt.deadline, RiskConfig: defaultRiskConfig(),
			}, persistence, runnerContextProvider(now), tt.ai, tt.execution, tt.syncer)
			runner.idGenerator = func() string { return "cycle-failure" }

			result, err := runner.RunCycle(context.Background())
			if err == nil {
				t.Fatal("expected cycle failure")
			}
			if result.State != paritydomain.CycleFailed || result.ErrorCode != tt.wantCode {
				t.Fatalf("result = %+v, want failed/%s", result, tt.wantCode)
			}
			if persistence.state("cycle-failure") != paritydomain.CycleFailed || persistence.errorCode != tt.wantCode {
				t.Fatalf("persisted state=%q code=%q", persistence.state("cycle-failure"), persistence.errorCode)
			}
		})
	}
}

func runnerContextProvider(now time.Time) ContextProvider {
	return &fakeContextProvider{
		account:    AccountSnapshot{TotalEquity: 100, AvailableBalance: 100},
		candidates: []CandidateSnapshot{{Symbol: "SOLUSDT", Score: 90}},
		markets:    map[string]MarketSnapshot{"SOLUSDT": validMarket("SOLUSDT", 100, now)},
	}
}

type fakeAIWorkflow struct {
	analysisErr error
	decision    DecisionInput
	decisionErr error
}

func (f fakeAIWorkflow) Analyze(context.Context, *TradingContextSnapshot) (AnalysisText, error) {
	return AnalysisText("analysis"), f.analysisErr
}

func (f fakeAIWorkflow) Decide(context.Context, AnalysisText, *TradingContextSnapshot) (DecisionInput, error) {
	return f.decision, f.decisionErr
}

type blockingAIWorkflow struct{}

func (blockingAIWorkflow) Analyze(ctx context.Context, _ *TradingContextSnapshot) (AnalysisText, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func (blockingAIWorkflow) Decide(context.Context, AnalysisText, *TradingContextSnapshot) (DecisionInput, error) {
	return DecisionInput{}, errors.New("unexpected decide")
}

type failingExecution struct{}

func (failingExecution) Execute(context.Context, DecisionInput) (ExecutionResult, error) {
	return ExecutionResult{}, errors.New("Bybit rejected order")
}

type noopSynchronizer struct{}

func (noopSynchronizer) Synchronize(context.Context, ExecutionResult) error { return nil }

type failingSynchronizer struct{}

func (failingSynchronizer) Synchronize(context.Context, ExecutionResult) error {
	return errors.New("fill history unavailable")
}

type fakeCyclePersistence struct {
	cycles       map[string]*paritydomain.Cycle
	errorCode    string
	errorMessage string
}

func newFakeCyclePersistence() *fakeCyclePersistence {
	return &fakeCyclePersistence{cycles: map[string]*paritydomain.Cycle{}}
}

func (f *fakeCyclePersistence) Create(cycle *paritydomain.Cycle, _ string, _ map[string]any) error {
	copyCycle := *cycle
	f.cycles[cycle.ID] = &copyCycle
	return nil
}

func (f *fakeCyclePersistence) Transition(id string, event paritydomain.CycleEvent) (*store.ParityCycleRecord, error) {
	cycle := f.cycles[id]
	if err := cycle.Transition(event); err != nil {
		return nil, err
	}
	return fakeCycleRecord(cycle), nil
}

func (f *fakeCyclePersistence) Complete(id string) (*store.ParityCycleRecord, error) {
	return f.Transition(id, paritydomain.EventComplete)
}

func (f *fakeCyclePersistence) Fail(id, code, message string) (*store.ParityCycleRecord, error) {
	record, err := f.Transition(id, paritydomain.EventFail)
	if err != nil {
		return nil, err
	}
	f.errorCode = code
	f.errorMessage = message
	record.ErrorCode = code
	record.ErrorMessage = message
	return record, nil
}

func (f *fakeCyclePersistence) state(id string) paritydomain.CycleState {
	return f.cycles[id].State
}

func fakeCycleRecord(cycle *paritydomain.Cycle) *store.ParityCycleRecord {
	return &store.ParityCycleRecord{ID: cycle.ID, AgentID: cycle.AgentID, State: cycle.State, Shadow: cycle.Shadow}
}
