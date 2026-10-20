// Package config loads and validates environment driven configuration.
// It contains no business rules: only defaults, parsing and validation.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the fully resolved runtime configuration.
type Config struct {
	AppEnv        string
	HTTPAddr      string
	DBPath        string
	LogLevel      string
	DevAPIEnabled bool

	LLMEnabled       bool
	LLMProvider      string
	LLMBaseURL       string
	LLMAPIKey        string
	LLMModel         string
	LLMTimeout       time.Duration
	LLMMaxToolRounds int
	LLMMaxToolCalls  int

	// Model-call logging. Kept as strings/ints here so the config package does not
	// depend on the ai package; cmd/server parses the level.
	LLMLogLevel     string
	LLMLogFile      string
	LLMLogBodyChars int

	ConversationRecentTurns int
	ConversationMaxChars    int
	PendingActionTTL        time.Duration
	ToolTimeout             time.Duration

	AfterSalesWindowDays int
	PriceProtectionDays  int

	// Admin console. The built-in account is only created when admin_users is
	// empty, so changing these values never rewrites an existing password.
	AdminUsername   string
	AdminPassword   string
	AdminSessionTTL time.Duration
}

// IsDev reports whether dev-only endpoints and the demo identity header are allowed.
func (c Config) IsDev() bool { return c.AppEnv == "dev" || c.AppEnv == "test" }

// DevAPIAllowed reports whether /api/v1/dev/* endpoints must be registered.
func (c Config) DevAPIAllowed() bool { return c.IsDev() && c.DevAPIEnabled }

// LLMMode reports which chat backend the process actually wires up. The web
// console shows it so nobody mistakes the rule-based demo model for a real one.
func (c Config) LLMMode() string {
	if c.LLMProvider == "offline" {
		return "offline"
	}
	if c.LLMEnabled {
		return "openai"
	}
	return "disabled"
}

// Load reads configuration from the process environment. A .env file in the
// working directory is used as a fallback for variables that are not already
// set; existing environment variables always win.
func Load() (Config, error) {
	LoadDotEnv(".env")

	cfg := Config{
		AppEnv:                  str("APP_ENV", "dev"),
		HTTPAddr:                str("HTTP_ADDR", ":8080"),
		DBPath:                  str("DB_PATH", "./data/ecommerce_support.db"),
		LogLevel:                str("LOG_LEVEL", "info"),
		DevAPIEnabled:           boolean("DEV_API_ENABLED", true),
		LLMEnabled:              boolean("LLM_ENABLED", false),
		LLMProvider:             str("LLM_PROVIDER", "openai"),
		LLMBaseURL:              strings.TrimRight(str("LLM_BASE_URL", ""), "/"),
		LLMAPIKey:               str("LLM_API_KEY", ""),
		LLMModel:                str("LLM_MODEL", ""),
		LLMTimeout:              seconds("LLM_TIMEOUT_SECONDS", 30),
		LLMMaxToolRounds:        integer("LLM_MAX_TOOL_ROUNDS", 5),
		LLMMaxToolCalls:         integer("LLM_MAX_TOOL_CALLS_PER_TURN", 8),
		LLMLogLevel:             str("LLM_LOG_LEVEL", "detail"),
		LLMLogFile:              str("LLM_LOG_FILE", "./data/llm_calls.jsonl"),
		LLMLogBodyChars:         integer("LLM_LOG_BODY_CHARS", 4000),
		ConversationRecentTurns: integer("CONVERSATION_RECENT_TURNS", 6),
		ConversationMaxChars:    integer("CONVERSATION_MAX_CONTEXT_CHARS", 40000),
		PendingActionTTL:        minutes("PENDING_ACTION_TTL_MINUTES", 10),
		ToolTimeout:             seconds("TOOL_TIMEOUT_SECONDS", 10),
		AfterSalesWindowDays:    integer("AFTER_SALES_WINDOW_DAYS", 7),
		PriceProtectionDays:     integer("PRICE_PROTECTION_WINDOW_DAYS", 15),
		AdminUsername:           str("ADMIN_USERNAME", "admin"),
		AdminPassword:           str("ADMIN_PASSWORD", defaultAdminPassword),
		AdminSessionTTL:         hours("ADMIN_SESSION_TTL_HOURS", 12),
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// defaultAdminPassword is the built-in console password for local development.
// Validate refuses to start outside dev/test while it is still in use, so the
// demo login can never quietly become a production one.
const defaultAdminPassword = "admin12345"

// IsDefaultAdminPassword reports whether the console still uses the shipped
// development password.
func (c Config) IsDefaultAdminPassword() bool { return c.AdminPassword == defaultAdminPassword }

// Validate reports the first configuration problem with an actionable message.
func (c Config) Validate() error {
	switch c.AppEnv {
	case "dev", "test", "staging", "prod":
	default:
		return fmt.Errorf("APP_ENV must be one of dev/test/staging/prod, got %q", c.AppEnv)
	}
	if c.HTTPAddr == "" {
		return fmt.Errorf("HTTP_ADDR must not be empty")
	}
	if c.DBPath == "" {
		return fmt.Errorf("DB_PATH must not be empty")
	}
	if !c.IsDev() && c.DevAPIEnabled {
		// Fail fast: dev endpoints must never be implicitly enabled in prod.
		return fmt.Errorf("DEV_API_ENABLED must be false when APP_ENV=%s", c.AppEnv)
	}
	provider := c.LLMProvider
	if provider == "" {
		provider = "openai"
	}
	switch provider {
	case "openai", "offline":
	default:
		return fmt.Errorf("LLM_PROVIDER must be openai or offline, got %q", c.LLMProvider)
	}
	if provider == "offline" && !c.IsDev() {
		// The offline backend is a keyword matcher for local demos only.
		return fmt.Errorf("LLM_PROVIDER=offline is a local demo backend and must not be used when APP_ENV=%s", c.AppEnv)
	}
	if c.LLMEnabled && provider == "openai" {
		if c.LLMBaseURL == "" {
			return fmt.Errorf("LLM_BASE_URL is required when LLM_ENABLED=true")
		}
		if c.LLMModel == "" {
			return fmt.Errorf("LLM_MODEL is required when LLM_ENABLED=true")
		}
	}
	if c.LLMMaxToolRounds <= 0 || c.LLMMaxToolRounds > 20 {
		return fmt.Errorf("LLM_MAX_TOOL_ROUNDS must be within 1..20, got %d", c.LLMMaxToolRounds)
	}
	if c.LLMMaxToolCalls <= 0 || c.LLMMaxToolCalls > 40 {
		return fmt.Errorf("LLM_MAX_TOOL_CALLS_PER_TURN must be within 1..40, got %d", c.LLMMaxToolCalls)
	}
	switch strings.ToLower(strings.TrimSpace(c.LLMLogLevel)) {
	case "", "off", "none", "disabled", "summary", "detail", "full":
	default:
		return fmt.Errorf("LLM_LOG_LEVEL must be off/summary/detail/full, got %q", c.LLMLogLevel)
	}
	if c.LLMLogBodyChars < 200 || c.LLMLogBodyChars > 200000 {
		return fmt.Errorf("LLM_LOG_BODY_CHARS must be within 200..200000, got %d", c.LLMLogBodyChars)
	}
	if c.ConversationRecentTurns < 1 {
		return fmt.Errorf("CONVERSATION_RECENT_TURNS must be >= 1, got %d", c.ConversationRecentTurns)
	}
	if c.ConversationMaxChars < 2000 {
		return fmt.Errorf("CONVERSATION_MAX_CONTEXT_CHARS must be >= 2000, got %d", c.ConversationMaxChars)
	}
	if c.PendingActionTTL <= 0 {
		return fmt.Errorf("PENDING_ACTION_TTL_MINUTES must be positive")
	}
	if strings.TrimSpace(c.AdminUsername) == "" {
		return fmt.Errorf("ADMIN_USERNAME must not be empty")
	}
	if len([]rune(c.AdminPassword)) < 8 {
		return fmt.Errorf("ADMIN_PASSWORD must be at least 8 characters")
	}
	if !c.IsDev() && c.IsDefaultAdminPassword() {
		return fmt.Errorf("ADMIN_PASSWORD must be changed from the development default when APP_ENV=%s", c.AppEnv)
	}
	if c.AdminSessionTTL <= 0 || c.AdminSessionTTL > 720*time.Hour {
		return fmt.Errorf("ADMIN_SESSION_TTL_HOURS must be within 1..720")
	}
	return nil
}

// LoadDotEnv reads KEY=VALUE lines from path and sets unset environment
// variables. Missing files are ignored. The file content is never logged.
func LoadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, value)
	}
}

func str(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func integer(key string, def int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return v
}

func boolean(key string, def bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return def
	}
	return v
}

func seconds(key string, def int) time.Duration {
	return time.Duration(integer(key, def)) * time.Second
}

func minutes(key string, def int) time.Duration {
	return time.Duration(integer(key, def)) * time.Minute
}

func hours(key string, def int) time.Duration {
	return time.Duration(integer(key, def)) * time.Hour
}
