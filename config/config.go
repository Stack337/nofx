package config

import (
	"fmt"
	"nofx/mcp"
	"nofx/telemetry"
	"os"
	"strconv"
	"strings"
	"time"
)

// insecureDefaultJWTSecret is the historical fallback value. Refusing to boot when
// JWT_SECRET matches it (or is missing) prevents the server from silently signing
// tokens with a well-known secret.
const insecureDefaultJWTSecret = "default-jwt-secret-change-in-production"

// minJWTSecretLength is the minimum byte length we accept for HS256 signing keys.
// HS256 keys shorter than 32 bytes are brute-forceable.
const minJWTSecretLength = 32

// Global configuration instance
var global *Config

// Config is the global configuration (loaded from .env)
// Only contains truly global config, trading related config is at trader/strategy level
type Config struct {
	// Service configuration
	APIServerPort int
	JWTSecret     string

	// Database configuration
	DBType     string // sqlite or postgres
	DBPath     string // SQLite database file path
	DBHost     string // PostgreSQL host
	DBPort     int    // PostgreSQL port
	DBUser     string // PostgreSQL user
	DBPassword string // PostgreSQL password
	DBName     string // PostgreSQL database name
	DBSSLMode  string // PostgreSQL SSL mode

	// Security configuration
	// TransportEncryption enables browser-side encryption for API keys
	// Requires HTTPS or localhost. Set to false for HTTP access via IP.
	TransportEncryption bool

	// Experience improvement (anonymous usage statistics)
	// Helps us understand product usage and improve the experience
	// Set EXPERIENCE_IMPROVEMENT=false to disable
	ExperienceImprovement bool

	// Market data provider API keys
	AlpacaAPIKey    string // Alpaca API key for US stocks
	AlpacaSecretKey string // Alpaca secret key
	TwelveDataKey   string // TwelveData API key for forex & metals

	AI500 AI500Config
}

type AI500Config struct {
	Enabled          bool
	Provider         string
	Model            string
	ScoreTTL         time.Duration
	FeatureFreshness time.Duration
	ObservationPath  string
	Symbols          []string
}

// MustInit initializes global configuration or panics. Use from main() so the
// process refuses to start under an insecure config (e.g. default JWT secret).
func MustInit() {
	if err := initConfig(); err != nil {
		panic(fmt.Sprintf("config: %v", err))
	}
}

// Init initializes global configuration (from .env). Prefer MustInit from main.
func Init() {
	if err := initConfig(); err != nil {
		// Preserve historical fail-soft behavior for non-main callers (tests, tools);
		// the process can still observe the error via Get() returning nil.
		fmt.Fprintf(os.Stderr, "config init failed: %v\n", err)
	}
}

func initConfig() error {
	cfg := &Config{
		APIServerPort:         8080,
		ExperienceImprovement: true, // Default: enabled to help improve the product
		// Database defaults
		DBType:    "sqlite",
		DBPath:    "data/data.db",
		DBHost:    "localhost",
		DBPort:    5432,
		DBUser:    "postgres",
		DBName:    "nofx",
		DBSSLMode: "disable",
	}

	// Load from environment variables
	if v := os.Getenv("JWT_SECRET"); v != "" {
		cfg.JWTSecret = strings.TrimSpace(v)
	}
	if cfg.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is required (set a random %d+ byte value in .env)", minJWTSecretLength)
	}
	if cfg.JWTSecret == insecureDefaultJWTSecret {
		return fmt.Errorf("JWT_SECRET matches the insecure default; generate a fresh random value (e.g. `openssl rand -base64 48`)")
	}
	if len(cfg.JWTSecret) < minJWTSecretLength {
		return fmt.Errorf("JWT_SECRET must be at least %d bytes (got %d); generate via `openssl rand -base64 48`", minJWTSecretLength, len(cfg.JWTSecret))
	}

	if v := os.Getenv("API_SERVER_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil && port > 0 {
			cfg.APIServerPort = port
		}
	}

	// Transport encryption: default false for easier deployment
	// Set TRANSPORT_ENCRYPTION=true to enable (requires HTTPS or localhost)
	if v := os.Getenv("TRANSPORT_ENCRYPTION"); v != "" {
		cfg.TransportEncryption = strings.ToLower(v) == "true"
	}

	// Experience improvement: anonymous usage statistics
	// Default enabled, set EXPERIENCE_IMPROVEMENT=false to disable
	if v := os.Getenv("EXPERIENCE_IMPROVEMENT"); v != "" {
		cfg.ExperienceImprovement = strings.ToLower(v) != "false"
	}

	// Market data provider API keys
	cfg.AlpacaAPIKey = os.Getenv("ALPACA_API_KEY")
	cfg.AlpacaSecretKey = os.Getenv("ALPACA_SECRET_KEY")
	cfg.TwelveDataKey = os.Getenv("TWELVEDATA_API_KEY")
	cfg.AI500 = loadAI500Config()

	// Database configuration
	if v := os.Getenv("DB_TYPE"); v != "" {
		cfg.DBType = strings.ToLower(v)
	}
	if v := os.Getenv("DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("DB_HOST"); v != "" {
		cfg.DBHost = v
	}
	if v := os.Getenv("DB_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil && port > 0 {
			cfg.DBPort = port
		}
	}
	if v := os.Getenv("DB_USER"); v != "" {
		cfg.DBUser = v
	}
	if v := os.Getenv("DB_PASSWORD"); v != "" {
		cfg.DBPassword = v
	}
	if v := os.Getenv("DB_NAME"); v != "" {
		cfg.DBName = v
	}
	if v := os.Getenv("DB_SSLMODE"); v != "" {
		cfg.DBSSLMode = v
	}

	global = cfg

	// Initialize experience improvement (installation ID will be set after database init)
	telemetry.Init(cfg.ExperienceImprovement, "")

	// Set up AI token usage tracking callback
	mcp.TokenUsageCallback = func(usage mcp.TokenUsage) {
		telemetry.TrackAIUsage(telemetry.AIUsageEvent{
			ModelProvider: usage.Provider,
			ModelName:     usage.Model,
			Channel:       usage.Channel(),
			InputTokens:   usage.PromptTokens,
			OutputTokens:  usage.CompletionTokens,
		})
	}
	return nil
}

func loadAI500Config() AI500Config {
	result := AI500Config{
		ScoreTTL:         10 * time.Minute,
		FeatureFreshness: 10 * time.Minute,
		ObservationPath:  "data/ai500/observations.jsonl",
	}
	result.Enabled = strings.EqualFold(strings.TrimSpace(os.Getenv("AI500_SHADOW_ENABLED")), "true")
	result.Provider = strings.TrimSpace(os.Getenv("AI500_PROVIDER"))
	result.Model = strings.TrimSpace(os.Getenv("AI500_MODEL"))
	if value := strings.TrimSpace(os.Getenv("AI500_SCORE_TTL")); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
			result.ScoreTTL = parsed
		}
	}
	if value := strings.TrimSpace(os.Getenv("AI500_FEATURE_FRESHNESS")); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
			result.FeatureFreshness = parsed
		}
	}
	if value := strings.TrimSpace(os.Getenv("AI500_OBSERVATION_PATH")); value != "" {
		result.ObservationPath = value
	}
	seen := map[string]struct{}{}
	for _, raw := range strings.Split(os.Getenv("AI500_SYMBOLS"), ",") {
		symbol := strings.ToUpper(strings.TrimSpace(raw))
		symbol = strings.NewReplacer("-", "", "_", "").Replace(symbol)
		if symbol == "" {
			continue
		}
		if !strings.HasSuffix(symbol, "USDT") {
			symbol += "USDT"
		}
		if _, exists := seen[symbol]; exists {
			continue
		}
		seen[symbol] = struct{}{}
		result.Symbols = append(result.Symbols, symbol)
	}
	return result
}

// Get returns the global configuration
func Get() *Config {
	if global == nil {
		Init()
	}
	return global
}
