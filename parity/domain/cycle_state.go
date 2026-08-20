package domain

import (
	"fmt"
	"time"
)

var cycleTransitions = map[CycleState]map[CycleEvent]CycleState{
	CycleScheduled: {
		EventCollectContext: CycleCollectingContext,
		EventFail:           CycleFailed,
	},
	CycleCollectingContext: {
		EventStartAnalysis: CycleAnalysisRound,
		EventFail:          CycleFailed,
	},
	CycleAnalysisRound: {
		EventStartToolRound: CycleToolRound,
		EventFail:           CycleFailed,
	},
	CycleToolRound: {
		EventStartValidation: CycleValidating,
		EventFail:            CycleFailed,
	},
	CycleValidating: {
		EventStartExecution: CycleExecuting,
		EventFail:           CycleFailed,
	},
	CycleExecuting: {
		EventStartSynchronization: CycleSynchronizing,
		EventFail:                 CycleFailed,
	},
	CycleSynchronizing: {
		EventComplete: CycleCompleted,
		EventFail:     CycleFailed,
	},
}

// TransitionError reports a rejected state transition without mutating Cycle.
type TransitionError struct {
	CycleID string
	From    CycleState
	Event   CycleEvent
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("cycle %s cannot apply event %q from state %q", e.CycleID, e.Event, e.From)
}

// Transition moves a cycle to the state selected by its transition table.
func (c *Cycle) Transition(event CycleEvent) error {
	stateEvents, ok := cycleTransitions[c.State]
	if !ok {
		return &TransitionError{CycleID: c.ID, From: c.State, Event: event}
	}

	next, ok := stateEvents[event]
	if !ok {
		return &TransitionError{CycleID: c.ID, From: c.State, Event: event}
	}

	c.State = next
	c.UpdatedAt = time.Now().UTC()
	return nil
}
