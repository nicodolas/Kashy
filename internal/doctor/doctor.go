// Package doctor checks which AI agents are configured to route through Kashy proxy.
package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// AgentStatus represents the connection status of an agent.
type AgentStatus struct {
	Name      string
	ConfigFile string
	Connected bool
	Details   string
}

// ProxyAddr is the address agents should point to.
const ProxyAddr = "http://localhost:4000"

// Check scans known agent configs and reports which ones are connected to Kashy.
func Check() []AgentStatus {
	var results []AgentStatus
	results = append(results, checkOMX()...)
	results = append(results, checkOpenCode()...)
	results = append(results, checkClaudeCode()...)
	return results
}

// checkOMX checks .codex/config.toml for openai_base_url pointing to proxy.
func checkOMX() []AgentStatus {
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".codex", "config.toml"),
	}
	// also check common project locations
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, ".codex", "config.toml"),
			filepath.Join(cwd, "oh-my-codex", ".codex", "config.toml"),
		)
	}

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		connected := strings.Contains(string(data), "localhost:4000")
		details := fmt.Sprintf("openai_base_url → %s", extractTOMLValue(string(data), "openai_base_url"))
		return []AgentStatus{{
			Name:       "OMX (oh-my-codex)",
			ConfigFile: path,
			Connected:  connected,
			Details:    details,
		}}
	}
	return []AgentStatus{{
		Name:       "OMX (oh-my-codex)",
		ConfigFile: "(not found)",
		Connected:  false,
		Details:    "config.toml not found in ~/.codex/ or project .codex/",
	}}
}

// checkOpenCode checks opencode.json for provider baseURL pointing to proxy.
func checkOpenCode() []AgentStatus {
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".config", "opencode", "opencode.json"),
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "opencode.json"))
	}

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		connected := strings.Contains(string(data), "localhost:4000")
		return []AgentStatus{{
			Name:       "OpenCode",
			ConfigFile: path,
			Connected:  connected,
			Details:    pointsToProxy(string(data)),
		}}
	}
	return []AgentStatus{{
		Name:       "OpenCode",
		ConfigFile: "(not found)",
		Connected:  false,
		Details:    "opencode.json not found",
	}}
}

// checkClaudeCode checks Claude Code settings for openAiBaseUrl, or Kiro MCP config for kashy server.
func checkClaudeCode() []AgentStatus {
	home, _ := os.UserHomeDir()

	// Check Kiro MCP config first
	kiroMCP := filepath.Join(home, ".kiro", "settings", "mcp.json")
	if data, err := os.ReadFile(kiroMCP); err == nil {
		connected := strings.Contains(string(data), "kashy.exe") || strings.Contains(string(data), `"kashy"`)
		detail := "Kashy MCP server not found in mcp.json"
		if connected {
			detail = "Kashy MCP server registered ✓"
		}
		return []AgentStatus{{
			Name:       "Kiro (MCP)",
			ConfigFile: kiroMCP,
			Connected:  connected,
			Details:    detail,
		}}
	}

	// Fallback: Claude Code settings.json with openAiBaseUrl
	var settingsPath string
	switch runtime.GOOS {
	case "windows":
		settingsPath = filepath.Join(home, "AppData", "Roaming", "Claude", "settings.json")
	case "darwin":
		settingsPath = filepath.Join(home, "Library", "Application Support", "Claude", "settings.json")
	default:
		settingsPath = filepath.Join(home, ".config", "claude", "settings.json")
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return []AgentStatus{{
			Name:       "Claude Code / Kiro",
			ConfigFile: "(not found)",
			Connected:  false,
			Details:    "settings.json not found",
		}}
	}

	connected := strings.Contains(string(data), "localhost:4000")
	return []AgentStatus{{
		Name:       "Claude Code",
		ConfigFile: settingsPath,
		Connected:  connected,
		Details:    pointsToProxy(string(data)),
	}}
}

// Report formats doctor results for terminal output.
func Report(statuses []AgentStatus) string {
	var sb strings.Builder
	sb.WriteString("Kashy Doctor — Agent Connection Report\n")
	sb.WriteString(strings.Repeat("─", 50) + "\n")
	for _, s := range statuses {
		icon := "✅"
		if !s.Connected {
			icon = "❌"
		}
		sb.WriteString(fmt.Sprintf("%s  %s\n", icon, s.Name))
		sb.WriteString(fmt.Sprintf("   Config: %s\n", s.ConfigFile))
		sb.WriteString(fmt.Sprintf("   Status: %s\n\n", s.Details))
	}
	sb.WriteString(fmt.Sprintf("To connect an agent, set its base URL to: %s/v1\n", ProxyAddr))

	// Show skills count from ~/.kashy/skills/
	skillsInfo := checkSkills()
	sb.WriteString("\n" + skillsInfo + "\n")
	return sb.String()
}

// checkSkills counts .md files in ~/.kashy/skills/.
func checkSkills() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".kashy", "skills")
	return checkSkillsFromDir(dir)
}

// checkSkillsFromDir counts .md files in the given directory (used for testing).
func checkSkillsFromDir(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Sprintf("📚 Skills: none (create ~/.kashy/skills/*.md to add skills)")
	}
	count := 0
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			count++
			names = append(names, strings.TrimSuffix(e.Name(), ".md"))
		}
	}
	if count == 0 {
		return "📚 Skills: none (create ~/.kashy/skills/*.md to add skills)"
	}
	return fmt.Sprintf("📚 Skills: %d loaded — %s", count, strings.Join(names, ", "))
}

// extractTOMLValue extracts a simple string value from TOML (no external dep).
func extractTOMLValue(content, key string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key) {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				return strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			}
		}
	}
	return "(not set)"
}

// pointsToProxy returns a human-readable connection status string.
func pointsToProxy(content string) string {
	if strings.Contains(content, "localhost:4000") {
		return "Points to Kashy proxy ✓"
	}
	return "Does NOT point to Kashy proxy — update base URL to http://localhost:4000/v1"
}

