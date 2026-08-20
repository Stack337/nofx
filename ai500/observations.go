package ai500

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"nofx/market"
)

var ErrObservationNotFound = errors.New("AI500 observation not found")

type OutcomeLabel struct {
	ObservationID  string    `json:"observation_id"`
	Horizon        string    `json:"horizon"`
	ReturnPct      float64   `json:"return_pct"`
	MaxDrawdownPct float64   `json:"max_drawdown_pct"`
	LabeledAt      time.Time `json:"labeled_at"`
}

type ObservationStore interface {
	Put(context.Context, ScoreObservation) error
	GetLatest(context.Context, string) (ScoreObservation, error)
	AddOutcome(context.Context, OutcomeLabel) error
}

type JSONLStore struct {
	path            string
	mu              sync.Mutex
	fingerprints    map[string]struct{}
	observations    map[string][]ScoreObservation
	allObservations []ScoreObservation
	outcomes        []OutcomeLabel
}

type observationRecord struct {
	Kind        string            `json:"kind"`
	Observation *ScoreObservation `json:"observation,omitempty"`
	Outcome     *OutcomeLabel     `json:"outcome,omitempty"`
}

func NewJSONLStore(path string) (*JSONLStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("AI500 observation path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create AI500 observation directory: %w", err)
	}
	_ = os.Chmod(filepath.Dir(path), 0o700)
	store := &JSONLStore{
		path: path, fingerprints: map[string]struct{}{},
		observations: map[string][]ScoreObservation{},
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *JSONLStore) Put(ctx context.Context, observation ScoreObservation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	observation.Symbol = market.Normalize(observation.Symbol)
	if observation.Symbol == "" {
		return errors.New("AI500 observation symbol is required")
	}
	fingerprint, err := observationFingerprint(observation)
	if err != nil {
		return err
	}
	observation.ID = fingerprint

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.fingerprints[fingerprint]; exists {
		return nil
	}
	record := observationRecord{Kind: "observation", Observation: &observation}
	if err := s.appendRecord(record); err != nil {
		return err
	}
	s.fingerprints[fingerprint] = struct{}{}
	s.observations[observation.Symbol] = append(s.observations[observation.Symbol], observation)
	s.allObservations = append(s.allObservations, observation)
	return nil
}

func (s *JSONLStore) GetLatest(ctx context.Context, symbol string) (ScoreObservation, error) {
	if err := ctx.Err(); err != nil {
		return ScoreObservation{}, err
	}
	symbol = strings.TrimSpace(symbol)
	if symbol != "" {
		symbol = market.Normalize(symbol)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.observations[symbol]
	if len(items) == 0 {
		return ScoreObservation{}, ErrObservationNotFound
	}
	return cloneObservation(items[len(items)-1]), nil
}

func (s *JSONLStore) ListObservations(ctx context.Context, symbol string, limit int) ([]ScoreObservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit < 1 {
		return []ScoreObservation{}, nil
	}
	symbol = strings.TrimSpace(symbol)
	if symbol != "" {
		symbol = market.Normalize(symbol)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.observations[symbol]
	if symbol == "" {
		items = s.allObservations
	}
	if len(items) > limit {
		items = items[len(items)-limit:]
	}
	result := make([]ScoreObservation, len(items))
	for index := range items {
		result[len(items)-1-index] = cloneObservation(items[index])
	}
	return result, nil
}

func (s *JSONLStore) ListOutcomes(ctx context.Context, limit int) ([]OutcomeLabel, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit < 1 {
		return []OutcomeLabel{}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	start := 0
	if len(s.outcomes) > limit {
		start = len(s.outcomes) - limit
	}
	result := append([]OutcomeLabel(nil), s.outcomes[start:]...)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result, nil
}

func (s *JSONLStore) AddOutcome(ctx context.Context, outcome OutcomeLabel) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(outcome.ObservationID) == "" || outcome.LabeledAt.IsZero() {
		return errors.New("AI500 outcome observation ID and timestamp are required")
	}
	validHorizon := outcome.Horizon == "15m" || outcome.Horizon == "1h" || outcome.Horizon == "4h" || outcome.Horizon == "24h"
	if !validHorizon {
		return fmt.Errorf("invalid AI500 outcome horizon %q", outcome.Horizon)
	}
	if !finite(outcome.ReturnPct) || !finite(outcome.MaxDrawdownPct) {
		return errors.New("AI500 outcome values must be finite")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.appendRecord(observationRecord{Kind: "outcome", Outcome: &outcome}); err != nil {
		return err
	}
	s.outcomes = append(s.outcomes, outcome)
	return nil
}

func (s *JSONLStore) load() error {
	file, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open AI500 observations: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var record observationRecord
		if json.Unmarshal(scanner.Bytes(), &record) != nil {
			continue
		}
		if record.Kind == "outcome" && record.Outcome != nil {
			s.outcomes = append(s.outcomes, *record.Outcome)
			continue
		}
		if record.Kind != "observation" || record.Observation == nil {
			continue
		}
		observation := cloneObservation(*record.Observation)
		if observation.ID == "" {
			continue
		}
		s.fingerprints[observation.ID] = struct{}{}
		s.observations[observation.Symbol] = append(s.observations[observation.Symbol], observation)
		s.allObservations = append(s.allObservations, observation)
		continue
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read AI500 observations: %w", err)
	}
	for symbol := range s.observations {
		sort.SliceStable(s.observations[symbol], func(i, j int) bool {
			return s.observations[symbol][i].CapturedAt.Before(s.observations[symbol][j].CapturedAt)
		})
	}
	sort.SliceStable(s.allObservations, func(i, j int) bool {
		return s.allObservations[i].CapturedAt.Before(s.allObservations[j].CapturedAt)
	})
	return nil
}

func (s *JSONLStore) appendRecord(record observationRecord) error {
	encoded, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal AI500 observation record: %w", err)
	}
	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open AI500 observation append log: %w", err)
	}
	defer file.Close()
	_ = file.Chmod(0o600)
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("append AI500 observation: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync AI500 observation log: %w", err)
	}
	return nil
}

func observationFingerprint(observation ScoreObservation) (string, error) {
	payload := struct {
		Symbol                                   string    `json:"symbol"`
		FeatureTimestamp                         time.Time `json:"feature_timestamp"`
		Model                                    string    `json:"model"`
		Score, LongScore, ShortScore, Confidence float64
		Regime                                   Regime `json:"regime"`
		SchemaVersion                            string `json:"schema_version"`
	}{
		Symbol: observation.Symbol, FeatureTimestamp: observation.FeatureTimestamp.UTC(),
		Model: observation.Model, Score: observation.Score, LongScore: observation.LongScore,
		ShortScore: observation.ShortScore, Confidence: observation.Confidence,
		Regime: observation.Regime, SchemaVersion: observation.SchemaVersion,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("fingerprint AI500 observation: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func cloneObservation(observation ScoreObservation) ScoreObservation {
	observation.Reasons = append([]string(nil), observation.Reasons...)
	return observation
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
