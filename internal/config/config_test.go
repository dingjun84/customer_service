package config_test

import (
	"testing"
	"time"

	"github.com/example/ecommerce-support/internal/config"
)

func TestValidateRejectsDevAPIInProd(t *testing.T) {
	cfg := baseConfig()
	cfg.AppEnv = "prod"
	cfg.DevAPIEnabled = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error: dev API must not be enabled in prod")
	}
}

func TestValidateRequiresLLMConfigWhenEnabled(t *testing.T) {
	cfg := baseConfig()
	cfg.LLMEnabled = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for missing LLM_BASE_URL")
	}
	cfg.LLMBaseURL = "https://api.example.com/v1"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for missing LLM_MODEL")
	}
	cfg.LLMModel = "demo-model"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBoundsToolLimits(t *testing.T) {
	cfg := baseConfig()
	cfg.LLMMaxToolRounds = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for zero max rounds")
	}
	cfg = baseConfig()
	cfg.ConversationMaxChars = 10
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for tiny context budget")
	}
}

func TestLoadReadsDefaultsAndEnv(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("LLM_ENABLED", "false")
	t.Setenv("DEV_API_ENABLED", "true")
	t.Setenv("PENDING_ACTION_TTL_MINUTES", "15")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.HTTPAddr != ":9090" {
		t.Fatalf("addr = %s", cfg.HTTPAddr)
	}
	if cfg.AppEnv != "test" || !cfg.IsDev() {
		t.Fatalf("app env = %s", cfg.AppEnv)
	}
	if cfg.PendingActionTTL.Minutes() != 15 {
		t.Fatalf("ttl = %v", cfg.PendingActionTTL)
	}
}

func baseConfig() config.Config {
	return config.Config{
		AppEnv: "test", HTTPAddr: ":8080", DBPath: "./data/test.db", LogLevel: "info",
		DevAPIEnabled: true, LLMEnabled: false,
		LLMTimeout: 30 * time.Second, LLMMaxToolRounds: 5, LLMMaxToolCalls: 8,
		ConversationRecentTurns: 6, ConversationMaxChars: 40000,
		PendingActionTTL: 10 * time.Minute, ToolTimeout: 10 * time.Second,
		AfterSalesWindowDays: 7, PriceProtectionDays: 15,
		AdminUsername: "admin", AdminPassword: "test-password-1", AdminSessionTTL: 12 * time.Hour,
	}
}

func TestValidateRejectsDefaultAdminPasswordOutsideDev(t *testing.T) {
	cfg := baseConfig()
	cfg.AppEnv = "prod"
	cfg.DevAPIEnabled = false
	cfg.AdminPassword = "admin12345"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error: shipped console password must not reach prod")
	}
	cfg.AdminPassword = "a-stronger-password"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
