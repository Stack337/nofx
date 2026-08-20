package audit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJSONLWriterAppendsEventsAndRedactsSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	w, err := NewJSONLWriter(path)
	if err != nil {
		t.Fatalf("NewJSONLWriter() error = %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	event := Event{
		ID: "event-1", Type: "agent.decision", AgentID: "agent-1", CycleID: "cycle-1",
		At: time.Unix(100, 0).UTC(),
		Fields: map[string]any{
			"action":     "hold",
			"api_key":    "must-not-appear",
			"raw_prompt": "must-not-appear-either",
		},
	}
	if err := w.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"action":"hold"`) || !strings.HasSuffix(text, "\n") {
		t.Fatalf("audit line = %q", text)
	}
	if strings.Contains(text, "must-not-appear") || strings.Contains(text, "raw_prompt") || strings.Contains(text, "api_key") {
		t.Fatalf("audit line leaked secret fields: %q", text)
	}
}
