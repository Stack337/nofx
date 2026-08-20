package parity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplayCapturedPromptAndShadowToolCall(t *testing.T) {
	t.Parallel()

	fixturePath := filepath.Join("..", "..", ".tools", "vergex-prompt-capture.json")
	fixture, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read prompt fixture: %v", err)
	}
	capture, err := ParsePromptCapture(fixture)
	if err != nil {
		t.Fatalf("parse prompt fixture: %v", err)
	}
	if capture.Model == "" || len(capture.Messages) != 2 {
		t.Fatalf("capture shape = %+v", capture)
	}

	decision, err := ReplayHoldToolCall(`{"symbol":"ALL","confidence":50}`)
	if err != nil {
		t.Fatalf("replay hold tool call: %v", err)
	}
	execution := NewShadowExecution()
	result, err := execution.Execute(t.Context(), decision)
	if err != nil {
		t.Fatalf("shadow execute: %v", err)
	}
	if !result.Simulated || execution.LiveRequests() != 0 {
		t.Fatalf("shadow result = %+v, live requests = %d", result, execution.LiveRequests())
	}
}
