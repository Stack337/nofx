package store

import (
	"encoding/json"
	"fmt"
	"time"

	paritydomain "nofx/parity/domain"

	"gorm.io/gorm"
)

// ParityCycleStore persists every state transition of the shadow-first runner.
type ParityCycleStore struct {
	db *gorm.DB
}

type parityCycleDB struct {
	ID            string     `gorm:"primaryKey;column:id"`
	AgentID       string     `gorm:"column:agent_id;not null;index:idx_parity_cycles_agent_started"`
	OwnerID       string     `gorm:"column:owner_id;not null;index:idx_parity_cycles_owner"`
	State         string     `gorm:"column:state;not null;index"`
	CorrelationID string     `gorm:"column:correlation_id;not null;uniqueIndex"`
	Shadow        bool       `gorm:"column:shadow;not null;default:true"`
	MetadataJSON  string     `gorm:"column:metadata_json;not null;default:'{}'"`
	ErrorCode     string     `gorm:"column:error_code;not null;default:''"`
	ErrorMessage  string     `gorm:"column:error_message;not null;default:''"`
	StartedAt     time.Time  `gorm:"column:started_at;not null;index:idx_parity_cycles_agent_started,sort:desc"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;not null"`
	CompletedAt   *time.Time `gorm:"column:completed_at"`
}

func (parityCycleDB) TableName() string { return "parity_cycles" }

// ParityCycleRecord is the durable representation returned to runners and APIs.
type ParityCycleRecord struct {
	ID            string
	AgentID       string
	OwnerID       string
	State         paritydomain.CycleState
	CorrelationID string
	Shadow        bool
	Metadata      map[string]any
	ErrorCode     string
	ErrorMessage  string
	StartedAt     time.Time
	UpdatedAt     time.Time
	CompletedAt   *time.Time
}

func NewParityCycleStore(db *gorm.DB) *ParityCycleStore {
	return &ParityCycleStore{db: db}
}

func (s *ParityCycleStore) initTables() error {
	return s.db.AutoMigrate(&parityCycleDB{})
}

// Create persists a scheduled cycle and its non-secret diagnostic metadata.
func (s *ParityCycleStore) Create(cycle *paritydomain.Cycle, correlationID string, metadata map[string]any) error {
	if cycle == nil {
		return fmt.Errorf("cycle is nil")
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode parity cycle metadata: %w", err)
	}

	record := &parityCycleDB{
		ID:            cycle.ID,
		AgentID:       cycle.AgentID,
		OwnerID:       cycle.OwnerID,
		State:         string(cycle.State),
		CorrelationID: correlationID,
		Shadow:        cycle.Shadow,
		MetadataJSON:  string(encoded),
		StartedAt:     cycle.CreatedAt.UTC(),
		UpdatedAt:     cycle.UpdatedAt.UTC(),
	}
	if err := s.db.Create(record).Error; err != nil {
		return fmt.Errorf("create parity cycle: %w", err)
	}
	return nil
}

// Transition atomically validates and persists one lifecycle event.
func (s *ParityCycleStore) Transition(id string, event paritydomain.CycleEvent) (*ParityCycleRecord, error) {
	var result *ParityCycleRecord
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var dbRecord parityCycleDB
		if err := tx.Where("id = ?", id).First(&dbRecord).Error; err != nil {
			return fmt.Errorf("load parity cycle: %w", err)
		}

		cycle := paritydomain.Cycle{
			ID:        dbRecord.ID,
			AgentID:   dbRecord.AgentID,
			OwnerID:   dbRecord.OwnerID,
			State:     paritydomain.CycleState(dbRecord.State),
			Shadow:    dbRecord.Shadow,
			CreatedAt: dbRecord.StartedAt,
			UpdatedAt: dbRecord.UpdatedAt,
		}
		if err := cycle.Transition(event); err != nil {
			return err
		}

		updates := map[string]any{
			"state":      string(cycle.State),
			"updated_at": cycle.UpdatedAt,
		}
		if cycle.State == paritydomain.CycleCompleted || cycle.State == paritydomain.CycleFailed {
			completedAt := cycle.UpdatedAt
			updates["completed_at"] = completedAt
			dbRecord.CompletedAt = &completedAt
		}
		if err := tx.Model(&parityCycleDB{}).Where("id = ? AND state = ?", id, dbRecord.State).Updates(updates).Error; err != nil {
			return fmt.Errorf("persist parity cycle transition: %w", err)
		}

		dbRecord.State = string(cycle.State)
		dbRecord.UpdatedAt = cycle.UpdatedAt
		result = parityRecordFromDB(&dbRecord)
		return nil
	})
	return result, err
}

// Complete transitions a synchronizing cycle to completed.
func (s *ParityCycleStore) Complete(id string) (*ParityCycleRecord, error) {
	return s.Transition(id, paritydomain.EventComplete)
}

// Fail transitions an active cycle to failed and records a stable error code.
func (s *ParityCycleStore) Fail(id, code, message string) (*ParityCycleRecord, error) {
	record, err := s.Transition(id, paritydomain.EventFail)
	if err != nil {
		return nil, err
	}
	if err := s.db.Model(&parityCycleDB{}).Where("id = ?", id).Updates(map[string]any{
		"error_code":    code,
		"error_message": message,
	}).Error; err != nil {
		return nil, fmt.Errorf("persist parity cycle failure: %w", err)
	}
	record.ErrorCode = code
	record.ErrorMessage = message
	return record, nil
}

// Get loads one persisted cycle.
func (s *ParityCycleStore) Get(id string) (*ParityCycleRecord, error) {
	var dbRecord parityCycleDB
	if err := s.db.Where("id = ?", id).First(&dbRecord).Error; err != nil {
		return nil, fmt.Errorf("get parity cycle: %w", err)
	}
	return parityRecordFromDB(&dbRecord), nil
}

// ListByAgent returns recent cycles for one agent without exposing other
// agents' state to the API layer.
func (s *ParityCycleStore) ListByAgent(agentID string, limit int) ([]*ParityCycleRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	var rows []*parityCycleDB
	if err := s.db.Where("agent_id = ?", agentID).
		Order("started_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list parity cycles: %w", err)
	}
	result := make([]*ParityCycleRecord, 0, len(rows))
	for _, row := range rows {
		result = append(result, parityRecordFromDB(row))
	}
	return result, nil
}

func parityRecordFromDB(dbRecord *parityCycleDB) *ParityCycleRecord {
	metadata := map[string]any{}
	_ = json.Unmarshal([]byte(dbRecord.MetadataJSON), &metadata)
	return &ParityCycleRecord{
		ID:            dbRecord.ID,
		AgentID:       dbRecord.AgentID,
		OwnerID:       dbRecord.OwnerID,
		State:         paritydomain.CycleState(dbRecord.State),
		CorrelationID: dbRecord.CorrelationID,
		Shadow:        dbRecord.Shadow,
		Metadata:      metadata,
		ErrorCode:     dbRecord.ErrorCode,
		ErrorMessage:  dbRecord.ErrorMessage,
		StartedAt:     dbRecord.StartedAt,
		UpdatedAt:     dbRecord.UpdatedAt,
		CompletedAt:   dbRecord.CompletedAt,
	}
}
