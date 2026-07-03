// Package nicoconfig manages Kashy's persistent user configuration.
// Config is stored at ~/.kashy/config.toml — editable by hand or via `kashy config` command.
package kashyconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds all user-configurable settings for Kashy.
type Config struct {
	Providers ProviderConfig `toml:"providers"`
	Budget    BudgetConfig   `toml:"budget"`
	Loop      LoopConfig     `toml:"loop"`
}

// ProviderConfig holds API keys and endpoints.
type ProviderConfig struct {
	OpenRouter OpenRouterConfig `toml:"openrouter"`
}

// OpenRouterConfig holds OpenRouter-specific settings.
type OpenRouterConfig struct {
	APIKey  string `toml:"api_key"`
	BaseURL string `toml:"base_url"`
}

// BudgetConfig holds cost limits.
type BudgetConfig struct {
	SessionHardStop float64 `toml:"session_hard_stop"` // USD
	WarnAt          float64 `toml:"warn_at"`            // fraction 0-1
	DailyLimit      float64 `toml:"daily_limit"`        // USD
}

// LoopConfig holds agent loop defaults.
type LoopConfig struct {
	DefaultModel string `toml:"default_model"`
	MaxIter      int    `toml:"max_iter"`
}

// defaults returns factory default config values.
func defaults() Config {
	return Config{
		Providers: ProviderConfig{
			OpenRouter: OpenRouterConfig{
				BaseURL: "https://openrouter.ai/api/v1",
			},
		},
		Budget: BudgetConfig{
			SessionHardStop: 1.00,
			WarnAt:          0.80,
			DailyLimit:      10.00,
		},
		Loop: LoopConfig{
			DefaultModel: "anthropic/claude-3-haiku",
			MaxIter:      50,
		},
	}
}

// configPath returns the config file path.
// If the KASHY_CONFIG env var is set, it is used directly.
// Otherwise falls back to ~/.kashy/config.toml.
func configPath() string {
	if p := os.Getenv("KASHY_CONFIG"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".kashy", "config.toml")
}

// Load reads ~/.kashy/config.toml, falling back to defaults for missing fields.
// ENV var OPENROUTER_API_KEY overrides the config file value if set.
func Load() Config {
	cfg := defaults()

	path := configPath()
	data, err := os.ReadFile(path)
	if err == nil {
		_ = toml.Unmarshal(data, &cfg)
	}

	// ENV var takes priority over config file
	if env := os.Getenv("OPENROUTER_API_KEY"); env != "" {
		cfg.Providers.OpenRouter.APIKey = env
	}
	if cfg.Providers.OpenRouter.BaseURL == "" {
		cfg.Providers.OpenRouter.BaseURL = "https://openrouter.ai/api/v1"
	}

	return cfg
}

// Save writes the config to ~/.kashy/config.toml.
func Save(cfg Config) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create config: %w", err)
	}
	defer f.Close()

	enc := toml.NewEncoder(f)
	return enc.Encode(cfg)
}

// SetAPIKey sets the OpenRouter API key and saves config.
func SetAPIKey(key string) error {
	cfg := Load()
	cfg.Providers.OpenRouter.APIKey = key
	return Save(cfg)
}

// GetAPIKey returns the effective API key (config file or ENV var).
func GetAPIKey() string {
	return Load().Providers.OpenRouter.APIKey
}

// ConfigFilePath returns the path to the config file for display.
func ConfigFilePath() string {
	return configPath()
}

// Show returns a human-readable summary of the config (masking sensitive values).
func Show(cfg Config) string {
	key := cfg.Providers.OpenRouter.APIKey
	maskedKey := "(not set)"
	if len(key) > 8 {
		maskedKey = key[:8] + "..." + key[len(key)-4:]
	} else if len(key) > 0 {
		maskedKey = "****"
	}

	return fmt.Sprintf(`Kashy Configuration — %s

[providers.openrouter]
  api_key  = %s
  base_url = %s

[budget]
  session_hard_stop = $%.2f
  warn_at           = %.0f%%
  daily_limit       = $%.2f

[loop]
  default_model = %s
  max_iter      = %d
`,
		configPath(),
		maskedKey,
		cfg.Providers.OpenRouter.BaseURL,
		cfg.Budget.SessionHardStop,
		cfg.Budget.WarnAt*100,
		cfg.Budget.DailyLimit,
		cfg.Loop.DefaultModel,
		cfg.Loop.MaxIter,
	)
}


