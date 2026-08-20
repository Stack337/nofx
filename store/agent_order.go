package store

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type agentIdempotencyDB struct {
	Key             string     `gorm:"primaryKey;column:idempotency_key"`
	ExchangeOrderID string     `gorm:"column:exchange_order_id;not null;default:''"`
	CreatedAt       time.Time  `gorm:"column:created_at;autoCreateTime"`
	CompletedAt     *time.Time `gorm:"column:completed_at"`
}

func (agentIdempotencyDB) TableName() string { return "agent_order_idempotency" }

type IdempotencyReservation struct {
	Key             string
	ExchangeOrderID string
	CreatedAt       time.Time
	CompletedAt     *time.Time
}

func (s *AgentStore) Reserve(ctx context.Context, key string) (bool, error) {
	record := agentIdempotencyDB{Key: key}
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
	if result.Error != nil {
		return false, fmt.Errorf("reserve idempotency key: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

func (s *AgentStore) CompleteReservation(ctx context.Context, key, exchangeOrderID string) error {
	now := time.Now().UTC()
	result := s.db.WithContext(ctx).Model(&agentIdempotencyDB{}).Where("idempotency_key = ?", key).Updates(map[string]any{
		"exchange_order_id": exchangeOrderID,
		"completed_at":      now,
	})
	if result.Error != nil {
		return fmt.Errorf("complete idempotency reservation: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *AgentStore) GetReservation(ctx context.Context, key string) (IdempotencyReservation, error) {
	var record agentIdempotencyDB
	if err := s.db.WithContext(ctx).Where("idempotency_key = ?", key).First(&record).Error; err != nil {
		return IdempotencyReservation{}, fmt.Errorf("get idempotency reservation: %w", err)
	}
	return IdempotencyReservation{
		Key: record.Key, ExchangeOrderID: record.ExchangeOrderID,
		CreatedAt: record.CreatedAt, CompletedAt: record.CompletedAt,
	}, nil
}
