package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	agentdomain "nofx/agent"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openAgentTestStore(t *testing.T) *Store {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "agent.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	s, err := NewFromGorm(gdb)
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	if err := s.Agent().initTables(); err != nil {
		t.Fatalf("initialize agent tables: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestAgentStorePersistsModeAndLiveConfirmation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := openAgentTestStore(t)
	want := agentdomain.Agent{
		ID: "agent-1", UserID: "user-1", Name: "flash", ExchangeID: "bybit-1",
		AIModelID: "deepseek-flash", Mode: agentdomain.ModeShadow, Enabled: true,
	}
	if err := s.Agent().Create(ctx, want); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := s.Agent().UpdateMode(ctx, want.ID, agentdomain.ModeLive, true); err != nil {
		t.Fatalf("UpdateMode() error = %v", err)
	}

	got, err := s.Agent().Get(ctx, want.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Mode != agentdomain.ModeLive || !got.LiveConfirmed {
		t.Fatalf("agent = %+v, want live with confirmation", got)
	}
	listed, err := s.Agent().List(ctx, "user-1")
	if err != nil || len(listed) != 1 || listed[0].ID != want.ID {
		t.Fatalf("List() = %+v, %v", listed, err)
	}
}

func TestAgentCycleTransitionUsesCompareAndSwap(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := openAgentTestStore(t)
	cycle := agentdomain.AgentCycle{
		ID: "cycle-1", AgentID: "agent-1", Status: agentdomain.CycleQueued,
		Deadline: time.Now().UTC().Add(time.Minute),
	}
	if err := s.Agent().CreateCycle(ctx, cycle); err != nil {
		t.Fatalf("CreateCycle() error = %v", err)
	}
	if err := s.Agent().TransitionCycle(ctx, cycle.ID, agentdomain.CycleQueued, agentdomain.CycleProcessing, ""); err != nil {
		t.Fatalf("TransitionCycle() error = %v", err)
	}
	err := s.Agent().TransitionCycle(ctx, cycle.ID, agentdomain.CycleQueued, agentdomain.CycleCompleted, "")
	if !errors.Is(err, agentdomain.ErrInvalidCycleTransition) {
		t.Fatalf("second TransitionCycle() error = %v, want ErrInvalidCycleTransition", err)
	}
	got, err := s.Agent().GetCycle(ctx, cycle.ID)
	if err != nil || got.Status != agentdomain.CycleProcessing {
		t.Fatalf("GetCycle() = %+v, %v", got, err)
	}
}

func TestAgentIdempotencyReservationIsUnique(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := openAgentTestStore(t)

	reserved, err := s.Agent().Reserve(ctx, "cycle-1:open:BTCUSDT")
	if err != nil || !reserved {
		t.Fatalf("first Reserve() = %v, %v", reserved, err)
	}
	reserved, err = s.Agent().Reserve(ctx, "cycle-1:open:BTCUSDT")
	if err != nil || reserved {
		t.Fatalf("second Reserve() = %v, %v, want false nil", reserved, err)
	}
	if err := s.Agent().CompleteReservation(ctx, "cycle-1:open:BTCUSDT", "bybit-order-1"); err != nil {
		t.Fatalf("CompleteReservation() error = %v", err)
	}
	got, err := s.Agent().GetReservation(ctx, "cycle-1:open:BTCUSDT")
	if err != nil || got.ExchangeOrderID != "bybit-order-1" || got.CompletedAt == nil {
		t.Fatalf("GetReservation() = %+v, %v", got, err)
	}
}
