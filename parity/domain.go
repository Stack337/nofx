package parity

import "time"

// CycleState is the persisted lifecycle state of one agent cycle.
type CycleState string

const (
	CycleScheduled         CycleState = "scheduled"
	CycleCollectingContext CycleState = "collecting_context"
	CycleAnalysisRound     CycleState = "analysis_round"
	CycleToolRound         CycleState = "tool_round"
	CycleValidating        CycleState = "validating"
	CycleExecuting         CycleState = "executing"
	CycleSynchronizing     CycleState = "synchronizing"
	CycleCompleted         CycleState = "completed"
	CycleFailed            CycleState = "failed"
)

// CycleEvent requests a state transition.
type CycleEvent string

const (
	EventCollectContext       CycleEvent = "collect_context"
	EventStartAnalysis        CycleEvent = "start_analysis"
	EventStartToolRound       CycleEvent = "start_tool_round"
	EventStartValidation      CycleEvent = "start_validation"
	EventStartExecution       CycleEvent = "start_execution"
	EventStartSynchronization CycleEvent = "start_synchronization"
	EventComplete             CycleEvent = "complete"
	EventFail                 CycleEvent = "fail"
)

// Cycle identifies one bounded agent run.
type Cycle struct {
	ID        string
	AgentID   string
	State     CycleState
	Shadow    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewCycle starts a cycle in the scheduled state.
func NewCycle(id, agentID string, shadow bool) *Cycle {
	now := time.Now().UTC()
	return &Cycle{
		ID:        id,
		AgentID:   agentID,
		State:     CycleScheduled,
		Shadow:    shadow,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
