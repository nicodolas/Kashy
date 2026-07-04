package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nicodolas/kashy/internal/doctor"
	"github.com/nicodolas/kashy/internal/kashyconfig"
	"github.com/spf13/cobra"
)

// cmdDoctor checks agent connections and optionally fixes them.
func cmdDoctor() *cobra.Command {
	var fix bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check which agents are connected to Kashy proxy",
		Run: func(cmd *cobra.Command, args []string) {
			if fix {
				applyFixes()
			}
			statuses := doctor.Check()
			fmt.Print(doctor.Report(statuses))
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "Auto-patch agent config files to connect to Kashy")
	return cmd
}

// applyFixes patches known agent config files to use Kashy proxy.
func applyFixes() {
	home, _ := os.UserHomeDir()
	proxyURL := "http://localhost:4000/v1"

	// Fix OMX — .codex/config.toml
	omxConfig := filepath.Join(home, ".codex", "config.toml")
	if data, err := os.ReadFile(omxConfig); err == nil {
		content := string(data)
		if !strings.Contains(content, "localhost:4000") {
			if strings.Contains(content, "openai_base_url") {
				lines := strings.Split(content, "\n")
				for i, line := range lines {
					if strings.HasPrefix(strings.TrimSpace(line), "openai_base_url") {
						lines[i] = fmt.Sprintf(`openai_base_url = "%s"`, proxyURL)
					}
				}
				content = strings.Join(lines, "\n")
			} else {
				content += fmt.Sprintf("\nopenai_base_url = \"%s\"\n", proxyURL)
			}
			os.WriteFile(omxConfig, []byte(content), 0644)
			fmt.Printf("[kashy] ✅ Patched OMX config: %s\n", omxConfig)
		}
	}

	// Fix Kiro MCP — inform user
	kiroMCP := filepath.Join(home, ".kiro", "settings", "mcp.json")
	if _, err := os.Stat(kiroMCP); err == nil {
		fmt.Printf("[kashy] ℹ️  Kiro MCP: already managed. Run 'kashy mcp' for MCP server config.\n")
	}

	// Fix Antigravity IDE — settings.json with openai.baseURL
	var antigravitySettings string
	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		antigravitySettings = filepath.Join(appData, "Antigravity", "User", "settings.json")
	case "darwin":
		antigravitySettings = filepath.Join(home, "Library", "Application Support", "Antigravity", "User", "settings.json")
	default:
		antigravitySettings = filepath.Join(home, ".config", "Antigravity", "User", "settings.json")
	}
	if err := patchAntigravitySettings(antigravitySettings, proxyURL); err == nil {
		fmt.Printf("[kashy] ✅ Patched Antigravity settings: %s\n", antigravitySettings)
	}

	// Fix Claude Code — settings.json with openAiBaseUrl
	var claudeSettings string
	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		claudeSettings = filepath.Join(appData, "Claude", "settings.json")
	case "darwin":
		claudeSettings = filepath.Join(home, "Library", "Application Support", "Claude", "settings.json")
	default:
		claudeSettings = filepath.Join(home, ".config", "claude", "settings.json")
	}
	if err := patchJSONSettings(claudeSettings, "openAiBaseUrl", proxyURL); err == nil {
		fmt.Printf("[kashy] ✅ Patched Claude Code settings: %s\n", claudeSettings)
	}

	fmt.Println("[kashy] Run 'kashy doctor' to verify connections.")
}

// migrateFromNico copies API key from ~/.nico/config.toml to ~/.kashy/config.toml
// if the kashy config doesn't have a key yet. One-time migration for nico→kashy users.
func migrateFromNico() {
	home, _ := os.UserHomeDir()
	kashyCfgPath := filepath.Join(home, ".kashy", "config.toml")
	nicoCfgPath := filepath.Join(home, ".nico", "config.toml")

	current := kashyconfig.Load()
	if current.Providers.OpenRouter.APIKey != "" {
		return
	}
	if _, err := os.Stat(nicoCfgPath); err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(kashyCfgPath), 0755); err != nil {
		return
	}
	data, err := os.ReadFile(nicoCfgPath)
	if err != nil {
		return
	}
	if err := os.WriteFile(kashyCfgPath, data, 0644); err != nil {
		return
	}
	fmt.Println("[kashy] ℹ️  Migrated config from ~/.nico/ — you're all set.")
}

// patchJSONSettings reads a JSON settings file, sets key=value, and writes back.
// Idempotent: skips if the key already has the correct value.
// Creates the file (and parent dirs) if it doesn't exist.
func patchJSONSettings(path, key, value string) error {
	settings := map[string]interface{}{}

	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &settings)
	}

	if cur, ok := settings[key].(string); ok && strings.Contains(cur, "localhost:4000") {
		return nil // already correct
	}

	settings[key] = value

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}

// patchAntigravitySettings patches "openai.baseURL" in Antigravity settings.json.
func patchAntigravitySettings(path, proxyURL string) error {
	return patchJSONSettings(path, "openai.baseURL", proxyURL)
}
