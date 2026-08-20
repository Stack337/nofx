package risk

import (
	"context"
	"errors"
	"testing"
)

type memorySwitchStore struct {
	states map[string]SwitchState
}

func (s *memorySwitchStore) GetState(_ context.Context, agentID string) (SwitchState, error) {
	return s.states[agentID], nil
}

func (s *memorySwitchStore) PutState(_ context.Context, state SwitchState) error {
	s.states[state.AgentID] = state
	return nil
}

func TestKillSwitchRequiresExplicitConfirmationToDisable(t *testing.T) {
	ctx := context.Background()
	store := &memorySwitchStore{states: map[string]SwitchState{}}
	switcher := NewKillSwitch(store)
	if err := switcher.Enable(ctx, "agent-1", "daily loss"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	enabled, err := switcher.Enabled(ctx, "agent-1")
	if err != nil || !enabled {
		t.Fatalf("Enabled() = %v, %v", enabled, err)
	}
	if err := switcher.Disable(ctx, "agent-1", "yes"); !errors.Is(err, ErrInvalidSwitchConfirmation) {
		t.Fatalf("Disable(wrong) error = %v", err)
	}
	if err := switcher.Disable(ctx, "agent-1", "DISABLE agent-1"); err != nil {
		t.Fatalf("Disable(confirmed) error = %v", err)
	}
	enabled, err = switcher.Enabled(ctx, "agent-1")
	if err != nil || enabled {
		t.Fatalf("Enabled() after disable = %v, %v", enabled, err)
	}
}
