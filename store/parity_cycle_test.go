package store

import (
	"path/filepath"
	"testing"
	"time"

	paritydomain "nofx/parity/domain"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openParityTestStore(t *testing.T, dbPath string) *Store {
	t.Helper()

	gdb, err := gorm.Open(sqlite.New(sqlite.Config{DriverName: "sqlite", DSN: dbPath}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	s, err := NewFromGorm(gdb)
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	if err := s.ParityCycle().initTables(); err != nil {
		t.Fatalf("initialize parity cycle table: %v", err)
	}
	return s
}

func TestParityCyclePersistence(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "parity.db")
	first := openParityTestStore(t, dbPath)

	cycle := paritydomain.NewCycle("cycle-1", "agent-1", true)
	if err := first.ParityCycle().Create(cycle, "corr-1", map[string]any{"model": "deepseek-v4-flash"}); err != nil {
		t.Fatalf("create cycle: %v", err)
	}
	if _, err := first.ParityCycle().Transition("cycle-1", paritydomain.EventCollectContext); err != nil {
		t.Fatalf("collect context transition: %v", err)
	}
	if _, err := first.ParityCycle().Transition("cycle-1", paritydomain.EventStartAnalysis); err != nil {
		t.Fatalf("analysis transition: %v", err)
	}
	if _, err := first.ParityCycle().Fail("cycle-1", "ai_timeout", "analysis deadline exceeded"); err != nil {
		t.Fatalf("fail cycle: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	second := openParityTestStore(t, dbPath)
	t.Cleanup(func() { _ = second.Close() })

	record, err := second.ParityCycle().Get("cycle-1")
	if err != nil {
		t.Fatalf("reload cycle: %v", err)
	}
	if record.State != paritydomain.CycleFailed {
		t.Fatalf("state = %q, want %q", record.State, paritydomain.CycleFailed)
	}
	if record.CorrelationID != "corr-1" || record.ErrorCode != "ai_timeout" || record.ErrorMessage != "analysis deadline exceeded" {
		t.Fatalf("unexpected persisted diagnostics: %+v", record)
	}
	if !record.Shadow || record.CompletedAt == nil || record.UpdatedAt.Before(record.StartedAt) {
		t.Fatalf("unexpected persisted lifecycle fields: %+v", record)
	}
	if record.Metadata["model"] != "deepseek-v4-flash" {
		t.Fatalf("metadata = %#v", record.Metadata)
	}
}

func TestParityCycleTransitionIsAtomic(t *testing.T) {
	t.Parallel()

	s := openParityTestStore(t, filepath.Join(t.TempDir(), "parity.db"))
	t.Cleanup(func() { _ = s.Close() })

	cycle := paritydomain.NewCycle("cycle-atomic", "agent-1", true)
	if err := s.ParityCycle().Create(cycle, "corr-atomic", nil); err != nil {
		t.Fatalf("create cycle: %v", err)
	}
	if _, err := s.ParityCycle().Transition("cycle-atomic", paritydomain.EventStartExecution); err == nil {
		t.Fatal("expected invalid transition error")
	}

	record, err := s.ParityCycle().Get("cycle-atomic")
	if err != nil {
		t.Fatalf("reload cycle: %v", err)
	}
	if record.State != paritydomain.CycleScheduled {
		t.Fatalf("invalid transition persisted state %q", record.State)
	}
}

func TestParityCycleListByAgentReturnsNewestFirstWithLimit(t *testing.T) {
	t.Parallel()

	s := openParityTestStore(t, filepath.Join(t.TempDir(), "parity.db"))
	t.Cleanup(func() { _ = s.Close() })

	first := paritydomain.NewCycle("cycle-old", "agent-1", true)
	second := paritydomain.NewCycle("cycle-new", "agent-1", true)
	other := paritydomain.NewCycle("cycle-other", "agent-2", true)
	first.CreatedAt = first.CreatedAt.Add(-2 * time.Minute)
	first.UpdatedAt = first.CreatedAt
	second.CreatedAt = second.CreatedAt.Add(-time.Minute)
	second.UpdatedAt = second.CreatedAt
	if err := s.ParityCycle().Create(first, "corr-old", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.ParityCycle().Create(second, "corr-new", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.ParityCycle().Create(other, "corr-other", nil); err != nil {
		t.Fatal(err)
	}

	records, err := s.ParityCycle().ListByAgent("agent-1", 1)
	if err != nil {
		t.Fatalf("list cycles: %v", err)
	}
	if len(records) != 1 || records[0].ID != "cycle-new" {
		t.Fatalf("records = %+v", records)
	}
}
