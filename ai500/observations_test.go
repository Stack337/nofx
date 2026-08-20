package ai500

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func storedObservation(now time.Time, symbol string, score float64) ScoreObservation {
	observation := validObservation(now)
	observation.Symbol = symbol
	observation.Score = score
	return observation
}

func TestObservationStoreDeduplicatesAndReturnsLatest(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "private", "observations.jsonl")
	store, err := NewJSONLStore(path)
	if err != nil {
		t.Fatalf("NewJSONLStore() error = %v", err)
	}
	first := storedObservation(now, "BTCUSDT", 70)
	second := storedObservation(now.Add(time.Minute), "BTCUSDT", 80)
	if err := store.Put(context.Background(), first); err != nil {
		t.Fatalf("Put(first) error = %v", err)
	}
	if err := store.Put(context.Background(), first); err != nil {
		t.Fatalf("Put(duplicate) error = %v", err)
	}
	if err := store.Put(context.Background(), second); err != nil {
		t.Fatalf("Put(second) error = %v", err)
	}
	latest, err := store.GetLatest(context.Background(), "btcusdt")
	if err != nil {
		t.Fatalf("GetLatest() error = %v", err)
	}
	if latest.Score != 80 || latest.ID == "" {
		t.Fatalf("latest = %+v", latest)
	}
	if lines := countLines(t, path); lines != 2 {
		t.Fatalf("line count = %d, want 2", lines)
	}

	reopened, err := NewJSONLStore(path)
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}
	if err := reopened.Put(context.Background(), second); err != nil {
		t.Fatalf("reopened duplicate error = %v", err)
	}
	if lines := countLines(t, path); lines != 2 {
		t.Fatalf("line count after restart = %d, want 2", lines)
	}
}

func TestOutcomeStoreValidatesHorizons(t *testing.T) {
	store, err := NewJSONLStore(filepath.Join(t.TempDir(), "observations.jsonl"))
	if err != nil {
		t.Fatalf("NewJSONLStore() error = %v", err)
	}
	valid := OutcomeLabel{ObservationID: "abc", Horizon: "1h", ReturnPct: 2.5, MaxDrawdownPct: -1, LabeledAt: time.Now().UTC()}
	if err := store.AddOutcome(context.Background(), valid); err != nil {
		t.Fatalf("AddOutcome(valid) error = %v", err)
	}
	valid.Horizon = "2h"
	if err := store.AddOutcome(context.Background(), valid); err == nil {
		t.Fatal("invalid horizon was accepted")
	}
}

func TestObservationStoreDoesNotPersistCredentialOrPromptFields(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "observations.jsonl")
	store, err := NewJSONLStore(path)
	if err != nil {
		t.Fatalf("NewJSONLStore() error = %v", err)
	}
	if err := store.Put(context.Background(), storedObservation(now, "BTCUSDT", 80)); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	lower := strings.ToLower(string(content))
	for _, forbidden := range []string{"api_key", "authorization", "raw_prompt", "raw_response"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("persisted data contains forbidden field %q", forbidden)
		}
	}
}

func TestObservationStoreSerializesConcurrentWrites(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "observations.jsonl")
	store, err := NewJSONLStore(path)
	if err != nil {
		t.Fatalf("NewJSONLStore() error = %v", err)
	}
	var wait sync.WaitGroup
	for i := 0; i < 20; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			observation := storedObservation(now.Add(time.Duration(index)*time.Second), "BTCUSDT", float64(index))
			if err := store.Put(context.Background(), observation); err != nil {
				t.Errorf("Put(%d) error = %v", index, err)
			}
		}(i)
	}
	wait.Wait()
	if lines := countLines(t, path); lines != 20 {
		t.Fatalf("line count = %d, want 20", lines)
	}
}

func TestObservationStoreListsAllObservationsAndOutcomesAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "observations.jsonl")
	store, err := NewJSONLStore(path)
	if err != nil {
		t.Fatalf("NewJSONLStore: %v", err)
	}
	first := storedObservation(time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC), "BTCUSDT", 70)
	second := storedObservation(time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC), "SOLUSDT", 80)
	if err := store.Put(context.Background(), first); err != nil {
		t.Fatalf("put first: %v", err)
	}
	if err := store.Put(context.Background(), second); err != nil {
		t.Fatalf("put second: %v", err)
	}
	latest, err := store.GetLatest(context.Background(), "SOLUSDT")
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	outcome := OutcomeLabel{ObservationID: latest.ID, Horizon: "1h", ReturnPct: 2, MaxDrawdownPct: -1, LabeledAt: time.Now().UTC()}
	if err := store.AddOutcome(context.Background(), outcome); err != nil {
		t.Fatalf("add outcome: %v", err)
	}

	reopened, err := NewJSONLStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	observations, err := reopened.ListObservations(context.Background(), "", 10)
	if err != nil || len(observations) != 2 || observations[0].Symbol != "SOLUSDT" {
		t.Fatalf("observations = %#v, error = %v", observations, err)
	}
	outcomes, err := reopened.ListOutcomes(context.Background(), 10)
	if err != nil || len(outcomes) != 1 || outcomes[0].ObservationID != latest.ID {
		t.Fatalf("outcomes = %#v, error = %v", outcomes, err)
	}
}

func countLines(t *testing.T, path string) int {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	return count
}
