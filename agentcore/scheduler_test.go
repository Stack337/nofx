package agentcore

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestSchedulerStopsAndPreventsOverlappingRuns(t *testing.T) {
	var running, maxRunning atomic.Int32
	scheduler := NewScheduler(5*time.Millisecond, func(ctx context.Context) {
		current := running.Add(1)
		if current > maxRunning.Load() {
			maxRunning.Store(current)
		}
		select {
		case <-time.After(20 * time.Millisecond):
		case <-ctx.Done():
		}
		running.Add(-1)
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { scheduler.Start(ctx); close(done) }()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not stop")
	}
	if maxRunning.Load() != 1 {
		t.Fatalf("max concurrent runs = %d", maxRunning.Load())
	}
}
