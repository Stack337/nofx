package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCycleAllowsQueuedProcessingCompleted(t *testing.T) {
	cycle, err := NewCycle(AgentCycle{ID: "cycle-1", AgentID: "agent-1", Status: CycleQueued})
	if err != nil {
		t.Fatalf("NewCycle() error = %v", err)
	}

	if err := cycle.Transition(CycleProcessing, ""); err != nil {
		t.Fatalf("Transition(processing) error = %v", err)
	}
	if err := cycle.Transition(CycleCompleted, ""); err != nil {
		t.Fatalf("Transition(completed) error = %v", err)
	}
	if got := cycle.Snapshot().Status; got != CycleCompleted {
		t.Fatalf("status = %q, want %q", got, CycleCompleted)
	}
}

func TestCycleRejectsSecondTerminalTransition(t *testing.T) {
	cycle, err := NewCycle(AgentCycle{ID: "cycle-1", AgentID: "agent-1", Status: CycleQueued})
	if err != nil {
		t.Fatalf("NewCycle() error = %v", err)
	}
	if err := cycle.Transition(CycleProcessing, ""); err != nil {
		t.Fatalf("Transition(processing) error = %v", err)
	}
	if err := cycle.Transition(CycleFailed, "provider_timeout"); err != nil {
		t.Fatalf("Transition(failed) error = %v", err)
	}

	err = cycle.Transition(CycleCompleted, "")
	if !errors.Is(err, ErrInvalidCycleTransition) {
		t.Fatalf("Transition(completed) error = %v, want ErrInvalidCycleTransition", err)
	}
	state := cycle.Snapshot()
	if state.Status != CycleFailed || state.ErrorCode != "provider_timeout" {
		t.Fatalf("state = %+v, want failed state preserved", state)
	}
}

func TestRunWithDeadlineFailsCycleInsteadOfLeavingProcessing(t *testing.T) {
	cycle, err := NewCycle(AgentCycle{
		ID:       "cycle-1",
		AgentID:  "agent-1",
		Status:   CycleQueued,
		Deadline: time.Now().Add(20 * time.Millisecond),
	})
	if err != nil {
		t.Fatalf("NewCycle() error = %v", err)
	}
	runner := CycleRunnerFunc(func(ctx context.Context, _ AgentCycle) error {
		<-ctx.Done()
		return ctx.Err()
	})

	err = RunWithDeadline(context.Background(), cycle, runner)
	if !errors.Is(err, ErrCycleTimeout) {
		t.Fatalf("RunWithDeadline() error = %v, want ErrCycleTimeout", err)
	}
	state := cycle.Snapshot()
	if state.Status != CycleFailed || state.ErrorCode != CycleTimeoutCode {
		t.Fatalf("state = %+v, want failed timeout state", state)
	}
}
