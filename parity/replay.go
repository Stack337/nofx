package parity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type PromptMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type PromptCapture struct {
	Messages []PromptMessage `json:"messages"`
	Model    string          `json:"model"`
}

var capturedSecretPattern = regexp.MustCompile(`(?i)(sk-[a-z0-9_-]{20,}|sk-or-v1-[a-z0-9]{20,}|eyJ[a-z0-9_-]{30,})`)

// ParsePromptCapture validates the non-secret prompt fixture used by parity
// replay. It intentionally accepts only the fields needed by the replay.
func ParsePromptCapture(data []byte) (PromptCapture, error) {
	var capture PromptCapture
	if err := json.Unmarshal(data, &capture); err != nil {
		return PromptCapture{}, fmt.Errorf("invalid prompt capture JSON: %w", err)
	}
	if strings.TrimSpace(capture.Model) == "" || len(capture.Messages) == 0 {
		return PromptCapture{}, fmt.Errorf("prompt capture has no model or messages")
	}
	for _, message := range capture.Messages {
		if strings.TrimSpace(message.Role) == "" || strings.TrimSpace(message.Content) == "" {
			return PromptCapture{}, fmt.Errorf("prompt capture contains an incomplete message")
		}
		if capturedSecretPattern.MatchString(message.Content) {
			return PromptCapture{}, fmt.Errorf("prompt capture contains a secret-like token")
		}
	}
	return capture, nil
}

// ReplayHoldToolCall creates the safe synthetic decision used by shadow replay.
func ReplayHoldToolCall(arguments string) (DecisionInput, error) {
	var payload struct {
		Symbol     string  `json:"symbol"`
		Confidence float64 `json:"confidence"`
	}
	decoder := json.NewDecoder(bytes.NewBufferString(arguments))
	if err := decoder.Decode(&payload); err != nil {
		return DecisionInput{}, fmt.Errorf("invalid hold tool arguments: %w", err)
	}
	if strings.TrimSpace(payload.Symbol) == "" {
		return DecisionInput{}, fmt.Errorf("hold tool requires symbol")
	}
	return DecisionInput{Symbol: strings.ToUpper(strings.TrimSpace(payload.Symbol)), Action: ActionHold}, nil
}
