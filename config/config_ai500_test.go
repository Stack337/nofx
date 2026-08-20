package config

import (
	"reflect"
	"testing"
	"time"
)

func TestAI500ConfigIsDisabledByDefault(t *testing.T) {
	setAI500TestEnvironment(t)
	if err := initConfig(); err != nil {
		t.Fatalf("init config: %v", err)
	}
	if global.AI500.Enabled {
		t.Fatal("AI500 shadow mode enabled by default")
	}
	if global.AI500.ScoreTTL != 10*time.Minute || global.AI500.FeatureFreshness != 10*time.Minute {
		t.Fatalf("AI500 durations = %+v", global.AI500)
	}
}

func TestAI500ConfigLoadsExplicitShadowSettings(t *testing.T) {
	setAI500TestEnvironment(t)
	t.Setenv("AI500_SHADOW_ENABLED", "true")
	t.Setenv("AI500_PROVIDER", "openrouter")
	t.Setenv("AI500_MODEL", "deepseek/deepseek-v4-flash-0731")
	t.Setenv("AI500_SCORE_TTL", "20m")
	t.Setenv("AI500_FEATURE_FRESHNESS", "5m")
	t.Setenv("AI500_OBSERVATION_PATH", "private/ai500.jsonl")
	t.Setenv("AI500_SYMBOLS", "btc, SOL-USDT,btc")
	if err := initConfig(); err != nil {
		t.Fatalf("init config: %v", err)
	}
	wantSymbols := []string{"BTCUSDT", "SOLUSDT"}
	if !global.AI500.Enabled || global.AI500.Provider != "openrouter" || global.AI500.Model != "deepseek/deepseek-v4-flash-0731" {
		t.Fatalf("AI500 config = %+v", global.AI500)
	}
	if global.AI500.ScoreTTL != 20*time.Minute || global.AI500.FeatureFreshness != 5*time.Minute || !reflect.DeepEqual(global.AI500.Symbols, wantSymbols) {
		t.Fatalf("AI500 config = %+v, want symbols %#v", global.AI500, wantSymbols)
	}
}

func setAI500TestEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	for _, key := range []string{"AI500_SHADOW_ENABLED", "AI500_PROVIDER", "AI500_MODEL", "AI500_SCORE_TTL", "AI500_FEATURE_FRESHNESS", "AI500_OBSERVATION_PATH", "AI500_SYMBOLS"} {
		t.Setenv(key, "")
	}
	t.Cleanup(func() { global = nil })
}
