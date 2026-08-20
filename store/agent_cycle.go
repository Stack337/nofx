package store

import (
	"context"
	"fmt"
	"time"

	agentdomain "nofx/agent"

	"gorm.io/gorm"
)

type agentCycleDB struct {
	ID        string    `gorm:"primaryKey;column:id"`
	AgentID   string    `gorm:"column:agent_id;not null;index:idx_agent_cycles_agent_created,priority:1"`
	Status    string    `gorm:"column:status;not null;index:idx_agent_cycles_status"`
	Attempt   int       `gorm:"column:attempt;not null;default:0"`
	Deadline  time.Time `gorm:"column:deadline;not null"`
	ErrorCode string    `gorm:"column:error_code;not null;default:''"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime;index:idx_agent_cycles_agent_created,priority:2,sort:desc"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (agentCycleDB) TableName() string { return "agent_cycles" }

func (s *AgentStore) CreateCycle(ctx context.Context, cycle agentdomain.AgentCycle) error {
	record := agentCycleDB{
		ID: cycle.ID, AgentID: cycle.AgentID, Status: string(cycle.Status), Attempt: cycle.Attempt,
		Deadline: cycle.Deadline, ErrorCode: cycle.ErrorCode,
	}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return fmt.Errorf("create agent cycle: %w", err)
	}
	return nil
}

func (s *AgentStore) TransitionCycle(ctx context.Context, id string, from, to agentdomain.CycleStatus, errorCode string) error {
	if !agentdomain.CanTransition(from, to) {
		return fmt.Errorf("%w: %s -> %s", agentdomain.ErrInvalidCycleTransition, from, to)
	}
	updates := map[string]any{"status": string(to), "error_code": errorCode}
	if to == agentdomain.CycleProcessing {
		updates["attempt"] = gorm.Expr("attempt + 1")
	}
	result := s.db.WithContext(ctx).Model(&agentCycleDB{}).Where("id = ? AND status = ?", id, string(from)).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("transition agent cycle: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("%w: persisted status changed", agentdomain.ErrInvalidCycleTransition)
	}
	return nil
}

func (s *AgentStore) GetCycle(ctx context.Context, id string) (agentdomain.AgentCycle, error) {
	var record agentCycleDB
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&record).Error; err != nil {
		return agentdomain.AgentCycle{}, fmt.Errorf("get agent cycle: %w", err)
	}
	return agentdomain.AgentCycle{
		ID: record.ID, AgentID: record.AgentID, Status: agentdomain.CycleStatus(record.Status),
		Attempt: record.Attempt, Deadline: record.Deadline, ErrorCode: record.ErrorCode,
	}, nil
}
