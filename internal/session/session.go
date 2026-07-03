package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// State is the current session state written to state.json.
type State struct {
	SessionID    string    `json:"session_id"`
	StartTime    time.Time `json:"start_time"`
	TotalCostUSD float64   `json:"total_cost_usd"`
	PromptTokens int       `json:"prompt_tokens"`
	CompTokens   int       `json:"completion_tokens"`
	CallCount    int       `json:"call_count"`
	LastModel    string    `json:"last_model"`
	LastUpdated  time.Time `json:"last_updated"`
}

// HistoryEntry is one line in history.jsonl.
type HistoryEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Model     string    `json:"model"`
	PromptTok int       `json:"prompt_tokens"`
	CompTok   int       `json:"completion_tokens"`
	CostUSD   float64   `json:"cost_usd"`
	SessionID string    `json:"session_id"`
}

// Store manages session state and history on disk.
type Store struct {
	mu          sync.Mutex
	statePath   string
	historyPath string
}

var (
	defaultOnce  sync.Once
	defaultStore *Store
)

// New creates a Store backed by the given directory (created if missing).
func New(dir string) *Store {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "kashy: warning: cannot create state dir %s: %v\n", dir, err)
	}
	return &Store{
		statePath:   filepath.Join(dir, "state.json"),
		historyPath: filepath.Join(dir, "history.jsonl"),
	}
}

// Default returns the singleton Store using ~/.kashy/.
func Default() *Store {
	defaultOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		defaultStore = New(filepath.Join(home, ".kashy"))
	})
	return defaultStore
}

// ReadState reads state.json. Returns zero State if file is missing or invalid.
func (s *Store) ReadState() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readStateLocked()
}

func (s *Store) readStateLocked() State {
	data, err := os.ReadFile(s.statePath)
	if err != nil {
		return State{}
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}
	}
	return st
}

// WriteState overwrites state.json with the given State.
func (s *Store) WriteState(st State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeStateLocked(st)
}

func (s *Store) writeStateLocked(st State) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("session: marshal state: %w", err)
	}
	return os.WriteFile(s.statePath, data, 0o644)
}

// UpdateCost atomically accumulates cost and token counts, then appends to history.
func (s *Store) UpdateCost(model string, promptTok, compTok int, costUSD float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.readStateLocked()
	if st.SessionID == "" {
		st.SessionID = fmt.Sprintf("session-%d", time.Now().Unix())
		st.StartTime = time.Now()
	}
	st.TotalCostUSD += costUSD
	st.PromptTokens += promptTok
	st.CompTokens += compTok
	st.CallCount++
	st.LastModel = model
	st.LastUpdated = time.Now()

	if err := s.writeStateLocked(st); err != nil {
		return err
	}

	// Append history entry
	entry := HistoryEntry{
		Timestamp: time.Now(),
		Model:     model,
		PromptTok: promptTok,
		CompTok:   compTok,
		CostUSD:   costUSD,
		SessionID: st.SessionID,
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("session: marshal history entry: %w", err)
	}
	f, err := os.OpenFile(s.historyPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("session: open history: %w", err)
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s\n", line)
	if err != nil {
		return err
	}

	// Auto-trim: keep last 1000 entries to prevent unbounded growth
	_ = s.trimHistoryLocked(1000)
	return nil
}

// TodayCostUSD sums the cost of all history entries from today (local date).
// Used by daily_limit enforcement in the proxy.
func (s *Store) TodayCostUSD() float64 {
	entries, err := s.ReadHistory(500)
	if err != nil || len(entries) == 0 {
		return 0
	}
	today := time.Now().Format("2006-01-02")
	total := 0.0
	for _, e := range entries {
		if e.Timestamp.Format("2006-01-02") == today {
			total += e.CostUSD
		}
	}
	return total
}

// TrimHistory keeps only the last `keepLast` entries in history.jsonl.
// No-op if file has fewer entries than keepLast.
func (s *Store) TrimHistory(keepLast int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.trimHistoryLocked(keepLast)
}

// trimHistoryLocked performs trimming without acquiring the lock (caller must hold it).
func (s *Store) trimHistoryLocked(keepLast int) error {
	data, err := os.ReadFile(s.historyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("session: read history for trim: %w", err)
	}

	lines := splitLines(string(data))
	var nonEmpty []string
	for _, l := range lines {
		if l != "" {
			nonEmpty = append(nonEmpty, l)
		}
	}

	if len(nonEmpty) <= keepLast {
		return nil
	}

	trimmed := nonEmpty[len(nonEmpty)-keepLast:]
	content := strings.Join(trimmed, "\n") + "\n"
	return os.WriteFile(s.historyPath, []byte(content), 0o644)
}

// ClearState resets state.json to an empty object.
func (s *Store) ClearState() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.WriteFile(s.statePath, []byte("{}\n"), 0o644)
}

// ReadHistory returns the last n entries from history.jsonl.
func (s *Store) ReadHistory(n int) ([]HistoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.historyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("session: read history: %w", err)
	}

	lines := splitLines(string(data))
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	entries := make([]HistoryEntry, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var e HistoryEntry
		if err := json.Unmarshal([]byte(line), &e); err == nil {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

// splitLines splits on \n, trimming \r, skipping empty trailing line.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
