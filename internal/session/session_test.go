package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	want := State{
		SessionID:    "test-001",
		TotalCostUSD: 0.042,
		PromptTokens: 1000,
		CompTokens:   500,
		CallCount:    3,
		LastModel:    "claude-3-haiku",
	}
	if err := s.WriteState(want); err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	got := s.ReadState()
	if got.SessionID != want.SessionID {
		t.Errorf("SessionID: got %q, want %q", got.SessionID, want.SessionID)
	}
	if got.TotalCostUSD != want.TotalCostUSD {
		t.Errorf("TotalCostUSD: got %f, want %f", got.TotalCostUSD, want.TotalCostUSD)
	}
	if got.PromptTokens != want.PromptTokens {
		t.Errorf("PromptTokens: got %d, want %d", got.PromptTokens, want.PromptTokens)
	}
}

func TestUpdateCostAccumulates(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	if err := s.UpdateCost("claude-haiku", 1000, 500, 0.001); err != nil {
		t.Fatalf("UpdateCost 1: %v", err)
	}
	if err := s.UpdateCost("claude-haiku", 2000, 1000, 0.002); err != nil {
		t.Fatalf("UpdateCost 2: %v", err)
	}

	st := s.ReadState()
	if st.CallCount != 2 {
		t.Errorf("CallCount: got %d, want 2", st.CallCount)
	}
	if st.PromptTokens != 3000 {
		t.Errorf("PromptTokens: got %d, want 3000", st.PromptTokens)
	}
	const wantCost = 0.003
	if st.TotalCostUSD < wantCost-0.0001 || st.TotalCostUSD > wantCost+0.0001 {
		t.Errorf("TotalCostUSD: got %f, want ~%f", st.TotalCostUSD, wantCost)
	}
}

func TestReadEmptyStateNoFile(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	// File không tồn tại → không panic, trả zero State
	st := s.ReadState()
	if st.SessionID != "" {
		t.Errorf("expected empty SessionID, got %q", st.SessionID)
	}
}

func TestClearState(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	if err := s.WriteState(State{SessionID: "to-clear", TotalCostUSD: 9.99}); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	if err := s.ClearState(); err != nil {
		t.Fatalf("ClearState: %v", err)
	}
	st := s.ReadState()
	if st.SessionID != "" {
		t.Errorf("after clear, SessionID should be empty, got %q", st.SessionID)
	}
}

func TestTrimHistory(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	// Write 15 entries
	for i := 0; i < 15; i++ {
		if err := s.UpdateCost("model", 100, 50, 0.001); err != nil {
			t.Fatalf("UpdateCost: %v", err)
		}
	}

	// Trim to 10
	if err := s.TrimHistory(10); err != nil {
		t.Fatalf("TrimHistory: %v", err)
	}

	entries, err := s.ReadHistory(100)
	if err != nil {
		t.Fatalf("ReadHistory: %v", err)
	}
	if len(entries) != 10 {
		t.Errorf("expected 10 entries after trim, got %d", len(entries))
	}
}

func TestTrimHistoryNoOp(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	// Write 5 entries
	for i := 0; i < 5; i++ {
		s.UpdateCost("model", 100, 50, 0.001)
	}

	// Trim to 10 — should be no-op
	if err := s.TrimHistory(10); err != nil {
		t.Fatalf("TrimHistory: %v", err)
	}

	entries, _ := s.ReadHistory(100)
	if len(entries) != 5 {
		t.Errorf("expected 5 entries (no-op), got %d", len(entries))
	}
}

func TestTodayCostUSD(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	// Ghi 3 entries hôm nay
	for i := 0; i < 3; i++ {
		if err := s.UpdateCost("model", 100, 50, 0.10); err != nil {
			t.Fatalf("UpdateCost: %v", err)
		}
	}

	got := s.TodayCostUSD()
	const want = 0.30
	if got < want-0.001 || got > want+0.001 {
		t.Errorf("TodayCostUSD: got %.6f, want ~%.6f", got, want)
	}
}

func TestTodayCostUSDEmptyHistory(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	if got := s.TodayCostUSD(); got != 0 {
		t.Errorf("expected 0 for empty history, got %f", got)
	}
}

func TestReadHistoryTruncation(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	// Ghi 10 entries
	for i := 0; i < 10; i++ {
		if err := s.UpdateCost("model", 100, 50, 0.001); err != nil {
			t.Fatalf("UpdateCost: %v", err)
		}
	}

	// Chỉ lấy 3 entries cuối
	entries, err := s.ReadHistory(3)
	if err != nil {
		t.Fatalf("ReadHistory: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
}

func TestSplitLinesCRLF(t *testing.T) {
	lines := splitLines("hello\r\nworld\r\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "hello" {
		t.Errorf("line[0]: got %q, want %q", lines[0], "hello")
	}
	if lines[1] != "world" {
		t.Errorf("line[1]: got %q, want %q", lines[1], "world")
	}
}

func TestSplitLinesNoTrailingNewline(t *testing.T) {
	lines := splitLines("a\nb")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "a" || lines[1] != "b" {
		t.Errorf("unexpected lines: %v", lines)
	}
}

func TestUpdateCostConcurrent(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = s.UpdateCost("model", 10, 5, 0.001)
		}()
	}
	wg.Wait()

	st := s.ReadState()
	if st.CallCount != n {
		t.Errorf("CallCount: got %d, want %d", st.CallCount, n)
	}
}

// TestTodayCostUSDExcludesYesterday verifies that TodayCostUSD only counts
// entries from today, ignoring entries written with a yesterday timestamp.
func TestTodayCostUSDExcludesYesterday(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	// Ghi thẳng một entry ngày hôm qua vào history.jsonl
	entry := HistoryEntry{
		Timestamp: time.Now().AddDate(0, 0, -1),
		Model:     "old",
		CostUSD:   5.0,
		SessionID: "x",
	}
	line, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	histPath := filepath.Join(dir, "history.jsonl")
	f, err := os.OpenFile(histPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("open history: %v", err)
	}
	if _, err := f.WriteString(string(line) + "\n"); err != nil {
		f.Close()
		t.Fatalf("write yesterday entry: %v", err)
	}
	f.Close()

	// Thêm entry hôm nay qua UpdateCost
	if err := s.UpdateCost("today-model", 0, 0, 1.5); err != nil {
		t.Fatalf("UpdateCost: %v", err)
	}

	// TodayCostUSD() phải == 1.5 (chỉ tính hôm nay, không tính hôm qua)
	got := s.TodayCostUSD()
	const want = 1.5
	if got < want-0.0001 || got > want+0.0001 {
		t.Errorf("TodayCostUSD: got %.6f, want %.6f (yesterday's 5.0 must be excluded)", got, want)
	}
}
