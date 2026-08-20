package agentcore

import (
	"context"
	"sync"
	"time"
)

type Scheduler struct {
	interval time.Duration
	run      func(context.Context)
	gate     chan struct{}
	once     sync.Once
}

func NewScheduler(interval time.Duration, run func(context.Context)) *Scheduler {
	if interval <= 0 {
		interval = time.Minute
	}
	return &Scheduler{interval: interval, run: run, gate: make(chan struct{}, 1)}
}

func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	s.trigger(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.trigger(ctx)
		}
	}
}

func (s *Scheduler) trigger(ctx context.Context) {
	select {
	case s.gate <- struct{}{}:
		go func() { defer func() { <-s.gate }(); s.run(ctx) }()
	default:
	}
}
