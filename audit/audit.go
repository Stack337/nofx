package audit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Event struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	AgentID string         `json:"agent_id,omitempty"`
	CycleID string         `json:"cycle_id,omitempty"`
	At      time.Time      `json:"at"`
	Fields  map[string]any `json:"fields,omitempty"`
}

type Writer interface {
	Append(context.Context, Event) error
}

type JSONLWriter struct {
	mu     sync.Mutex
	file   *os.File
	closed bool
}

func NewJSONLWriter(path string) (*JSONLWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &JSONLWriter{file: file}, nil
}

func (w *JSONLWriter) Append(ctx context.Context, event Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return os.ErrClosed
	}
	event.Fields = sanitizeMap(event.Fields)
	return json.NewEncoder(w.file).Encode(event)
}

func (w *JSONLWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	return w.file.Close()
}

func sanitizeMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	result := make(map[string]any, len(input))
	for key, value := range input {
		if sensitiveKey(key) {
			continue
		}
		switch typed := value.(type) {
		case map[string]any:
			result[key] = sanitizeMap(typed)
		case []any:
			values := make([]any, len(typed))
			for i, item := range typed {
				if nested, ok := item.(map[string]any); ok {
					values[i] = sanitizeMap(nested)
				} else {
					values[i] = item
				}
			}
			result[key] = values
		default:
			result[key] = value
		}
	}
	return result
}

func sensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	blocked := []string{"api_key", "password", "secret", "token", "authorization", "raw_prompt", "refresh"}
	for _, value := range blocked {
		if normalized == value || strings.Contains(normalized, value) {
			return true
		}
	}
	return false
}
