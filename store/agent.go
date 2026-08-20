package store

import (
	"context"
	"fmt"
	"time"

	agentdomain "nofx/agent"
	"nofx/risk"

	"gorm.io/gorm"
)

type AgentStore struct {
	db *gorm.DB
}

type agentDB struct {
	ID            string    `gorm:"primaryKey;column:id"`
	UserID        string    `gorm:"column:user_id;not null;index:idx_ai_agents_user_created,priority:1"`
	Name          string    `gorm:"column:name;not null"`
	ExchangeID    string    `gorm:"column:exchange_id;not null;index"`
	AIModelID     string    `gorm:"column:ai_model_id;not null"`
	StrategyID    string    `gorm:"column:strategy_id;not null;default:''"`
	RiskProfileID string    `gorm:"column:risk_profile_id;not null;default:''"`
	Mode          string    `gorm:"column:mode;not null;default:shadow"`
	LiveConfirmed bool      `gorm:"column:live_confirmed;not null;default:false"`
	Enabled       bool      `gorm:"column:enabled;not null;default:false"`
	Schedule      string    `gorm:"column:schedule;not null;default:''"`
	KillSwitch    bool      `gorm:"column:kill_switch_enabled;not null;default:false"`
	KillReason    string    `gorm:"column:kill_switch_reason;not null;default:''"`
	KillChangedAt time.Time `gorm:"column:kill_switch_changed_at"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime;index:idx_ai_agents_user_created,priority:2,sort:desc"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (agentDB) TableName() string { return "ai_agents" }

func NewAgentStore(db *gorm.DB) *AgentStore {
	return &AgentStore{db: db}
}

func (s *AgentStore) InitTables() error {
	return s.initTables()
}

func (s *AgentStore) initTables() error {
	return s.db.AutoMigrate(&agentDB{}, &agentCycleDB{}, &agentIdempotencyDB{})
}

func (s *AgentStore) Create(ctx context.Context, value agentdomain.Agent) error {
	record := agentDBFromDomain(value)
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return fmt.Errorf("create agent: %w", err)
	}
	return nil
}

func (s *AgentStore) Get(ctx context.Context, id string) (agentdomain.Agent, error) {
	var record agentDB
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&record).Error; err != nil {
		return agentdomain.Agent{}, fmt.Errorf("get agent: %w", err)
	}
	return agentFromDB(record), nil
}

func (s *AgentStore) List(ctx context.Context, userID string) ([]agentdomain.Agent, error) {
	var records []agentDB
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	result := make([]agentdomain.Agent, 0, len(records))
	for _, record := range records {
		result = append(result, agentFromDB(record))
	}
	return result, nil
}

func (s *AgentStore) UpdateMode(ctx context.Context, id string, mode agentdomain.Mode, liveConfirmed bool) error {
	result := s.db.WithContext(ctx).Model(&agentDB{}).Where("id = ?", id).Updates(map[string]any{
		"mode":           string(mode),
		"live_confirmed": liveConfirmed,
	})
	if result.Error != nil {
		return fmt.Errorf("update agent mode: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func agentDBFromDomain(value agentdomain.Agent) agentDB {
	return agentDB{
		ID: value.ID, UserID: value.UserID, Name: value.Name, ExchangeID: value.ExchangeID,
		AIModelID: value.AIModelID, StrategyID: value.StrategyID, RiskProfileID: value.RiskProfileID,
		Mode: string(value.Mode), LiveConfirmed: value.LiveConfirmed, Enabled: value.Enabled,
		Schedule: value.Schedule, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func (s *AgentStore) GetState(ctx context.Context, agentID string) (risk.SwitchState, error) {
	var record agentDB
	if err := s.db.WithContext(ctx).Where("id = ?", agentID).First(&record).Error; err != nil {
		return risk.SwitchState{}, fmt.Errorf("get kill switch: %w", err)
	}
	return risk.SwitchState{AgentID: record.ID, Enabled: record.KillSwitch, Reason: record.KillReason, ChangedAt: record.KillChangedAt}, nil
}

func (s *AgentStore) PutState(ctx context.Context, state risk.SwitchState) error {
	result := s.db.WithContext(ctx).Model(&agentDB{}).Where("id = ?", state.AgentID).Updates(map[string]any{
		"kill_switch_enabled":    state.Enabled,
		"kill_switch_reason":     state.Reason,
		"kill_switch_changed_at": state.ChangedAt,
	})
	if result.Error != nil {
		return fmt.Errorf("persist kill switch: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func agentFromDB(record agentDB) agentdomain.Agent {
	return agentdomain.Agent{
		ID: record.ID, UserID: record.UserID, Name: record.Name, ExchangeID: record.ExchangeID,
		AIModelID: record.AIModelID, StrategyID: record.StrategyID, RiskProfileID: record.RiskProfileID,
		Mode: agentdomain.Mode(record.Mode), LiveConfirmed: record.LiveConfirmed, Enabled: record.Enabled,
		Schedule: record.Schedule, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}
