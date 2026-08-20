package risk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrInvalidSwitchConfirmation = errors.New("invalid kill switch confirmation")

type SwitchState struct {
	AgentID   string
	Enabled   bool
	Reason    string
	ChangedAt time.Time
}

type SwitchStore interface {
	GetState(context.Context, string) (SwitchState, error)
	PutState(context.Context, SwitchState) error
}

type KillSwitch struct {
	store SwitchStore
	now   func() time.Time
}

func NewKillSwitch(store SwitchStore) *KillSwitch {
	return &KillSwitch{store: store, now: time.Now}
}

func (s *KillSwitch) Enabled(ctx context.Context, agentID string) (bool, error) {
	state, err := s.store.GetState(ctx, agentID)
	return state.Enabled, err
}

func (s *KillSwitch) Enable(ctx context.Context, agentID, reason string) error {
	return s.store.PutState(ctx, SwitchState{
		AgentID: agentID, Enabled: true, Reason: strings.TrimSpace(reason), ChangedAt: s.now().UTC(),
	})
}

func (s *KillSwitch) Disable(ctx context.Context, agentID, confirmation string) error {
	if confirmation != fmt.Sprintf("DISABLE %s", agentID) {
		return ErrInvalidSwitchConfirmation
	}
	return s.store.PutState(ctx, SwitchState{AgentID: agentID, Enabled: false, ChangedAt: s.now().UTC()})
}
