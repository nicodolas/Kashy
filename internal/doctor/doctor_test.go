package doctor

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestExtractTOMLValueFound: tìm thấy key → trả value đúng
func TestExtractTOMLValueFound(t *testing.T) {
	content := `openai_base_url = "http://localhost:4000/v1"`
	got := extractTOMLValue(content, "openai_base_url")
	if got != "http://localhost:4000/v1" {
		t.Errorf("got %q, want %q", got, "http://localhost:4000/v1")
	}
}

// TestExtractTOMLValueNotFound: key không tồn tại → "(not set)"
func TestExtractTOMLValueNotFound(t *testing.T) {
	content := `other = "x"`
	got := extractTOMLValue(content, "missing_key")
	if got != "(not set)" {
		t.Errorf("got %q, want %q", got, "(not set)")
	}
}

// TestExtractTOMLValueSingleQuote: single quote cũng strip đúng
func TestExtractTOMLValueSingleQuote(t *testing.T) {
	content := "key = 'myvalue'"
	got := extractTOMLValue(content, "key")
	if got != "myvalue" {
		t.Errorf("got %q, want %q", got, "myvalue")
	}
}

// TestExtractTOMLValueMultiline: nhiều keys, lấy đúng key
func TestExtractTOMLValueMultiline(t *testing.T) {
	content := "model = \"gpt-4\"\nopenai_base_url = \"http://localhost:4000/v1\"\ntemp = \"0.7\""
	got := extractTOMLValue(content, "openai_base_url")
	if got != "http://localhost:4000/v1" {
		t.Errorf("got %q, want %q", got, "http://localhost:4000/v1")
	}
}

// TestPointsToProxyTrue: content có "localhost:4000" → chứa "✓" hoặc "proxy"
func TestPointsToProxyTrue(t *testing.T) {
	got := pointsToProxy(`{"baseURL": "http://localhost:4000/v1"}`)
	lower := strings.ToLower(got)
	if !strings.Contains(lower, "proxy") && !strings.Contains(got, "✓") {
		t.Errorf("expected 'proxy' or '✓' in output, got: %q", got)
	}
}

// TestPointsToProxyFalse: content không có "localhost:4000" → chứa "NOT"
func TestPointsToProxyFalse(t *testing.T) {
	got := pointsToProxy(`{"baseURL": "https://api.openai.com/v1"}`)
	if !strings.Contains(got, "NOT") {
		t.Errorf("expected 'NOT' in output, got: %q", got)
	}
}

// TestReport: mix connected/not → chứa "✅" và "❌"
func TestReport(t *testing.T) {
	statuses := []AgentStatus{
		{Name: "AgentA", ConfigFile: "/a", Connected: true, Details: "ok"},
		{Name: "AgentB", ConfigFile: "/b", Connected: false, Details: "missing"},
	}
	got := Report(statuses)
	if !strings.Contains(got, "✅") {
		t.Errorf("expected '✅' in report, got: %s", got)
	}
	if !strings.Contains(got, "❌") {
		t.Errorf("expected '❌' in report, got: %s", got)
	}
	if !strings.Contains(got, "AgentA") {
		t.Errorf("expected 'AgentA' in report, got: %s", got)
	}
	if !strings.Contains(got, "AgentB") {
		t.Errorf("expected 'AgentB' in report, got: %s", got)
	}
}

// TestCheckSkillsNoDir: home dir trỏ đến tempdir không có skills/ → "none"
func TestCheckSkillsNoDir(t *testing.T) {
	tmpHome := t.TempDir()
	setHomeEnv(t, tmpHome)

	got := checkSkills()
	if !strings.Contains(got, "none") {
		t.Errorf("expected 'none' when no skills dir, got: %q", got)
	}
}

// TestCheckSkillsWithMdFiles: tạo skills/ với 2 .md files → "2 loaded"
func TestCheckSkillsWithMdFiles(t *testing.T) {
	tmpHome := t.TempDir()
	setHomeEnv(t, tmpHome)

	// Tạo ~/.kashy/skills/ với 2 .md files
	skillsDir := filepath.Join(tmpHome, ".kashy", "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	os.WriteFile(filepath.Join(skillsDir, "skill1.md"), []byte("# Skill 1"), 0644)
	os.WriteFile(filepath.Join(skillsDir, "skill2.md"), []byte("# Skill 2"), 0644)
	// Thêm một file không phải .md để đảm bảo không bị đếm
	os.WriteFile(filepath.Join(skillsDir, "ignore.txt"), []byte("not a skill"), 0644)

	got := checkSkills()
	if !strings.Contains(got, "2 loaded") {
		t.Errorf("expected '2 loaded', got: %q", got)
	}
}

// setHomeEnv set HOME (Linux/Mac) hoặc USERPROFILE (Windows) để override os.UserHomeDir()
func setHomeEnv(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", dir)
	} else {
		t.Setenv("HOME", dir)
	}
}

// --- checkKiro tests ---

func TestCheckKiroNotFound(t *testing.T) {
	tmpHome := t.TempDir()
	setHomeEnv(t, tmpHome)

	statuses := checkKiro()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Connected {
		t.Error("expected Connected=false when mcp.json absent")
	}
	if statuses[0].ConfigFile != "(not found)" {
		t.Errorf("expected (not found), got %q", statuses[0].ConfigFile)
	}
}

func TestCheckKiroConnected(t *testing.T) {
	tmpHome := t.TempDir()
	setHomeEnv(t, tmpHome)

	mcpPath := filepath.Join(tmpHome, ".kiro", "settings", "mcp.json")
	os.MkdirAll(filepath.Dir(mcpPath), 0755)
	os.WriteFile(mcpPath, []byte(`{"mcpServers":{"kashy":{"command":"kashy","args":["mcp"]}}}`), 0644)

	statuses := checkKiro()
	if !statuses[0].Connected {
		t.Errorf("expected Connected=true, got false — Details: %q", statuses[0].Details)
	}
	if statuses[0].Name != "Kiro (MCP)" {
		t.Errorf("expected Name='Kiro (MCP)', got %q", statuses[0].Name)
	}
}

func TestCheckKiroNotConnected(t *testing.T) {
	tmpHome := t.TempDir()
	setHomeEnv(t, tmpHome)

	mcpPath := filepath.Join(tmpHome, ".kiro", "settings", "mcp.json")
	os.MkdirAll(filepath.Dir(mcpPath), 0755)
	os.WriteFile(mcpPath, []byte(`{"mcpServers":{"other":{"command":"other"}}}`), 0644)

	statuses := checkKiro()
	if statuses[0].Connected {
		t.Error("expected Connected=false when kashy not in mcp.json")
	}
}

// --- checkClaudeCode tests ---

func TestCheckClaudeCodeNotFound(t *testing.T) {
	tmpHome := t.TempDir()
	setHomeEnv(t, tmpHome)
	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", filepath.Join(tmpHome, "AppData", "Roaming"))
	}

	statuses := checkClaudeCode()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Connected {
		t.Error("expected Connected=false when settings.json absent")
	}
}

// TestCheckAntigravityNotFound: no settings.json → Connected=false, "(not found)"
func TestCheckAntigravityNotFound(t *testing.T) {
	tmpHome := t.TempDir()
	setHomeEnv(t, tmpHome)
	// Also clear APPDATA on Windows so the function uses the tmpHome-derived path.
	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", filepath.Join(tmpHome, "AppData", "Roaming"))
	}

	statuses := checkAntigravity()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if s.Connected {
		t.Error("expected Connected=false when settings.json absent")
	}
	if s.ConfigFile != "(not found)" {
		t.Errorf("expected ConfigFile=(not found), got %q", s.ConfigFile)
	}
	if !strings.Contains(s.Details, "not found") {
		t.Errorf("expected 'not found' in Details, got %q", s.Details)
	}
}

// TestCheckAntigravityConnected: settings.json has localhost:4000 → Connected=true
func TestCheckAntigravityConnected(t *testing.T) {
	tmpHome := t.TempDir()
	setHomeEnv(t, tmpHome)

	settingsPath := antigravitySettingsPath(t, tmpHome)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := `{"openai.baseURL": "http://localhost:4000/v1"}`
	if err := os.WriteFile(settingsPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	statuses := checkAntigravity()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if !s.Connected {
		t.Errorf("expected Connected=true, got false — Details: %q", s.Details)
	}
	if s.Name != "Antigravity IDE" {
		t.Errorf("expected Name='Antigravity IDE', got %q", s.Name)
	}
}

// TestCheckAntigravityNotConnected: settings.json exists but points elsewhere → Connected=false
func TestCheckAntigravityNotConnected(t *testing.T) {
	tmpHome := t.TempDir()
	setHomeEnv(t, tmpHome)

	settingsPath := antigravitySettingsPath(t, tmpHome)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := `{"openai.baseURL": "https://api.openai.com/v1"}`
	if err := os.WriteFile(settingsPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	statuses := checkAntigravity()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if s.Connected {
		t.Error("expected Connected=false when URL is not localhost:4000")
	}
	if !strings.Contains(s.Details, "NOT") {
		t.Errorf("expected 'NOT' in Details, got %q", s.Details)
	}
}

// antigravitySettingsPath returns the OS-specific Antigravity settings.json path
// rooted at the given tmpHome, mirroring the logic in checkAntigravity().
func antigravitySettingsPath(t *testing.T, tmpHome string) string {
	t.Helper()
	switch runtime.GOOS {
	case "windows":
		appData := filepath.Join(tmpHome, "AppData", "Roaming")
		t.Setenv("APPDATA", appData)
		return filepath.Join(appData, "Antigravity", "User", "settings.json")
	case "darwin":
		return filepath.Join(tmpHome, "Library", "Application Support", "Antigravity", "User", "settings.json")
	default:
		return filepath.Join(tmpHome, ".config", "Antigravity", "User", "settings.json")
	}
}
