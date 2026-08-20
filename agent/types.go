package agent

import (
	"context"
	"time"
)

type Mode string

const (
	ModeShadow Mode = "shadow"
	ModePaper  Mode = "paper"
	ModeLive   Mode = "live"
)

type CycleStatus string

const (
	CycleQueued              CycleStatus = "queued"
	CycleProcessing          CycleStatus = "processing"
	CycleCompleted           CycleStatus = "completed"
	CycleFailed              CycleStatus = "failed"
	CycleNeedsReconciliation CycleStatus = "needs_reconciliation"
	CycleStopped             CycleStatus = "stopped"
)

type DecisionAction string

const (
	DecisionOpen  DecisionAction = "open"
	DecisionClose DecisionAction = "close"
	DecisionHold  DecisionAction = "hold"
)

type AgentCycle struct {
	ID        string
	AgentID   string
	Status    CycleStatus
	Attempt   int
	Deadline  time.Time
	ErrorCode string
}

type Agent struct {
	ID            string
	UserID        string
	Name          string
	ExchangeID    string
	AIModelID     string
	StrategyID    string
	RiskProfileID string
	Mode          Mode
	LiveConfirmed bool
	Enabled       bool
	Schedule      string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type CycleRunner interface {
	Run(context.Context, AgentCycle) error
}

type CycleRunnerFunc func(context.Context, AgentCycle) error

func (f CycleRunnerFunc) Run(ctx context.Context, cycle AgentCycle) error {
	return f(ctx, cycle)
}
