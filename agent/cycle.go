package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

const (
	CycleTimeoutCode  = "cycle_timeout"
	cycleCanceledCode = "cycle_canceled"
	cycleRunnerCode   = "cycle_runner_failed"
)

var (
	ErrInvalidCycleTransition = errors.New("invalid cycle transition")
	ErrInvalidCycle           = errors.New("invalid cycle")
	ErrCycleTimeout           = errors.New("cycle deadline exceeded")
)

type Cycle struct {
	mu    sync.Mutex
	state AgentCycle
}

func NewCycle(state AgentCycle) (*Cycle, error) {
	if state.ID == "" || state.AgentID == "" || state.Status != CycleQueued {
		return nil, ErrInvalidCycle
	}
	return &Cycle{state: state}, nil
}

func (c *Cycle) Snapshot() AgentCycle {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

func (c *Cycle) Transition(to CycleStatus, errorCode string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	from := c.state.Status
	if !validTransition(from, to) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidCycleTransition, from, to)
	}
	if to == CycleProcessing {
		c.state.Attempt++
	}
	c.state.Status = to
	c.state.ErrorCode = errorCode
	return nil
}

func validTransition(from, to CycleStatus) bool {
	if from == CycleQueued {
		return to == CycleProcessing
	}
	if from != CycleProcessing {
		return false
	}
	switch to {
	case CycleCompleted, CycleFailed, CycleNeedsReconciliation, CycleStopped:
		return true
	default:
		return false
	}
}

func CanTransition(from, to CycleStatus) bool {
	return validTransition(from, to)
}

func RunWithDeadline(parent context.Context, cycle *Cycle, runner CycleRunner) error {
	if err := cycle.Transition(CycleProcessing, ""); err != nil {
		return err
	}

	state := cycle.Snapshot()
	ctx := parent
	cancel := func() {}
	if !state.Deadline.IsZero() {
		ctx, cancel = context.WithDeadline(parent, state.Deadline)
	}
	defer cancel()

	err := runner.Run(ctx, state)
	if err == nil {
		return cycle.Transition(CycleCompleted, "")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		if transitionErr := cycle.Transition(CycleFailed, CycleTimeoutCode); transitionErr != nil {
			return errors.Join(ErrCycleTimeout, transitionErr)
		}
		return ErrCycleTimeout
	}
	if errors.Is(err, context.Canceled) {
		if transitionErr := cycle.Transition(CycleStopped, cycleCanceledCode); transitionErr != nil {
			return errors.Join(err, transitionErr)
		}
		return err
	}
	if transitionErr := cycle.Transition(CycleFailed, cycleRunnerCode); transitionErr != nil {
		return errors.Join(err, transitionErr)
	}
	return err
}
