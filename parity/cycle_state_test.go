package parity

import (
	"errors"
	"testing"
	"time"
)

func TestCycleTransitionHappyPath(t *testing.T) {
	t.Parallel()

	cycle := NewCycle("cycle-1", "agent-1", true)
	wantStates := []CycleState{
		CycleCollectingContext,
		CycleAnalysisRound,
		CycleToolRound,
		CycleValidating,
		CycleExecuting,
		CycleSynchronizing,
		CycleCompleted,
	}
	events := []CycleEvent{
		EventCollectContext,
		EventStartAnalysis,
		EventStartToolRound,
		EventStartValidation,
		EventStartExecution,
		EventStartSynchronization,
		EventComplete,
	}

	for i, event := range events {
		if err := cycle.Transition(event); err != nil {
			t.Fatalf("transition %q failed: %v", event, err)
		}
		if cycle.State != wantStates[i] {
			t.Fatalf("after %q state = %q, want %q", event, cycle.State, wantStates[i])
		}
	}
}

func TestCycleTransitionRejectsSkippedState(t *testing.T) {
	t.Parallel()

	cycle := NewCycle("cycle-2", "agent-1", true)
	err := cycle.Transition(EventStartAnalysis)
	if err == nil {
		t.Fatal("expected an invalid transition error")
	}

	var transitionErr *TransitionError
	if !errors.As(err, &transitionErr) {
		t.Fatalf("error type = %T, want *TransitionError", err)
	}
	if transitionErr.CycleID != "cycle-2" || transitionErr.From != CycleScheduled || transitionErr.Event != EventStartAnalysis {
		t.Fatalf("unexpected transition error: %+v", transitionErr)
	}
	if cycle.State != CycleScheduled {
		t.Fatalf("invalid transition changed state to %q", cycle.State)
	}
}

func TestCycleCanFailFromEveryActiveState(t *testing.T) {
	t.Parallel()

	activeStates := []CycleState{
		CycleScheduled,
		CycleCollectingContext,
		CycleAnalysisRound,
		CycleToolRound,
		CycleValidating,
		CycleExecuting,
		CycleSynchronizing,
	}

	for _, state := range activeStates {
		state := state
		t.Run(string(state), func(t *testing.T) {
			t.Parallel()
			cycle := Cycle{ID: "cycle-fail", AgentID: "agent-1", State: state, Shadow: true}
			if err := cycle.Transition(EventFail); err != nil {
				t.Fatalf("failed transition from %q: %v", state, err)
			}
			if cycle.State != CycleFailed {
				t.Fatalf("state = %q, want %q", cycle.State, CycleFailed)
			}
		})
	}
}

func TestTerminalCycleRejectsFurtherTransitions(t *testing.T) {
	t.Parallel()

	for _, state := range []CycleState{CycleCompleted, CycleFailed} {
		cycle := Cycle{ID: "cycle-terminal", AgentID: "agent-1", State: state, Shadow: true}
		if err := cycle.Transition(EventFail); err == nil {
			t.Fatalf("terminal state %q accepted another transition", state)
		}
	}
}

func TestCycleTransitionUpdatesTimestamp(t *testing.T) {
	t.Parallel()

	old := time.Date(2026, time.August, 20, 0, 0, 0, 0, time.UTC)
	cycle := Cycle{ID: "cycle-time", AgentID: "agent-1", State: CycleScheduled, Shadow: true, UpdatedAt: old}

	if err := cycle.Transition(EventCollectContext); err != nil {
		t.Fatalf("transition failed: %v", err)
	}
	if !cycle.UpdatedAt.After(old) {
		t.Fatalf("updated_at = %s, want a value after %s", cycle.UpdatedAt, old)
	}
	if cycle.UpdatedAt.Location() != time.UTC {
		t.Fatalf("updated_at location = %s, want UTC", cycle.UpdatedAt.Location())
	}
}
