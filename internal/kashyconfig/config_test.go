package kashyconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadDefaults: no file, no KASHY_CONFIG env var → returns defaults
func TestLoadDefaults(t *testing.T) {
	// Point KASHY_CONFIG at a non-existent file in a temp dir so we never
	// touch ~/.kashy/config.toml and Load() falls back to defaults().
	t.Setenv("KASHY_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	t.Setenv("OPENROUTER_API_KEY", "") // make sure env override is cleared

	cfg := Load()

	if cfg.Budget.SessionHardStop != 1.0 {
		t.Errorf("SessionHardStop: got %v, want 1.0", cfg.Budget.SessionHardStop)
	}
	if cfg.Budget.WarnAt != 0.8 {
		t.Errorf("WarnAt: got %v, want 0.8", cfg.Budget.WarnAt)
	}
	if cfg.Budget.DailyLimit != 10.0 {
		t.Errorf("DailyLimit: got %v, want 10.0", cfg.Budget.DailyLimit)
	}
	if cfg.Loop.DefaultModel != "anthropic/claude-3-haiku" {
		t.Errorf("DefaultModel: got %q, want %q", cfg.Loop.DefaultModel, "anthropic/claude-3-haiku")
	}
}

// TestSaveAndLoadRoundtrip: Save then Load preserves values.
func TestSaveAndLoadRoundtrip(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("KASHY_CONFIG", cfgPath)
	t.Setenv("OPENROUTER_API_KEY", "") // prevent env from overriding the saved key

	cfg := defaults()
	cfg.Providers.OpenRouter.APIKey = "sk-test-roundtrip"
	cfg.Budget.SessionHardStop = 2.5

	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	got := Load()
	if got.Providers.OpenRouter.APIKey != "sk-test-roundtrip" {
		t.Errorf("APIKey: got %q, want %q", got.Providers.OpenRouter.APIKey, "sk-test-roundtrip")
	}
	if got.Budget.SessionHardStop != 2.5 {
		t.Errorf("SessionHardStop: got %v, want 2.5", got.Budget.SessionHardStop)
	}
}

// TestKashyConfigEnvOverride: ConfigFilePath() returns KASHY_CONFIG when set.
func TestKashyConfigEnvOverride(t *testing.T) {
	customPath := filepath.Join(t.TempDir(), "custom_config.toml")
	t.Setenv("KASHY_CONFIG", customPath)

	got := ConfigFilePath()
	if got != customPath {
		t.Errorf("ConfigFilePath(): got %q, want %q", got, customPath)
	}
	if strings.Contains(got, ".kashy") {
		t.Errorf("ConfigFilePath() should not reference ~/.kashy when KASHY_CONFIG is set, got %q", got)
	}
}

// TestOpenRouterAPIKeyEnvOverride: OPENROUTER_API_KEY overrides config file.
func TestOpenRouterAPIKeyEnvOverride(t *testing.T) {
	// Point at non-existent file so Load() uses defaults (APIKey = "")
	t.Setenv("KASHY_CONFIG", filepath.Join(t.TempDir(), "nonexistent.toml"))
	t.Setenv("OPENROUTER_API_KEY", "env-override-key")

	got := GetAPIKey()
	if got != "env-override-key" {
		t.Errorf("GetAPIKey(): got %q, want %q", got, "env-override-key")
	}
}

// TestShowMaskingLongKey: key > 8 chars → "first8...last4"
func TestShowMaskingLongKey(t *testing.T) {
	cfg := defaults()
	cfg.Providers.OpenRouter.APIKey = "sk-or-valid-key-1234"

	output := Show(cfg)
	if !strings.Contains(output, "sk-or-va...1234") {
		t.Errorf("Show() masking wrong for long key, got:\n%s", output)
	}
}

// TestShowMaskingShortKey: key ≤ 8 chars → "****"
func TestShowMaskingShortKey(t *testing.T) {
	cfg := defaults()
	cfg.Providers.OpenRouter.APIKey = "abcd"

	output := Show(cfg)
	if !strings.Contains(output, "****") {
		t.Errorf("Show() masking wrong for short key, got:\n%s", output)
	}
}

// TestShowMaskingEmpty: key = "" → "(not set)"
func TestShowMaskingEmpty(t *testing.T) {
	cfg := defaults()
	cfg.Providers.OpenRouter.APIKey = ""

	output := Show(cfg)
	if !strings.Contains(output, "(not set)") {
		t.Errorf("Show() should show (not set) for empty key, got:\n%s", output)
	}
}

// TestSetAPIKeyAndGet: SetAPIKey persists the key, GetAPIKey returns it.
func TestSetAPIKeyAndGet(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("KASHY_CONFIG", cfgPath)
	t.Setenv("OPENROUTER_API_KEY", "") // prevent env override

	if err := SetAPIKey("my-new-key"); err != nil {
		t.Fatalf("SetAPIKey() error: %v", err)
	}

	got := GetAPIKey()
	if got != "my-new-key" {
		t.Errorf("GetAPIKey(): got %q, want %q", got, "my-new-key")
	}
}

// Ensure the "os" import isn't flagged unused — it's used via t.Setenv which
// internally calls os.Setenv, but the import is also directly referenced below
// to keep the compiler happy if t.Setenv is the only usage.
var _ = os.Getenv
