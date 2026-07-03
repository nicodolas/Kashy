package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nicodolas/kashy/internal/session"
)

func TestStatusEndpoint(t *testing.T) {
	store := session.New(t.TempDir())
	srv := New(store)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var st session.State
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
}

func TestHistoryEndpointEmpty(t *testing.T) {
	store := session.New(t.TempDir())
	srv := New(store)

	req := httptest.NewRequest(http.MethodGet, "/history", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var entries []session.HistoryEntry
	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestResetBudget(t *testing.T) {
	store := session.New(t.TempDir())
	store.UpdateCost("model", 100, 50, 0.5)
	srv := New(store)

	req := httptest.NewRequest(http.MethodPost, "/reset-budget", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	st := store.ReadState()
	if st.TotalCostUSD != 0 {
		t.Errorf("expected 0 cost after reset, got %f", st.TotalCostUSD)
	}
}

func TestVerifyEndpointNoTestSuite(t *testing.T) {
	store := session.New(t.TempDir())
	srv := New(store)

	// Use json.Marshal to correctly escape the temp dir path (handles Windows backslashes)
	type reqBody struct {
		TargetDir string `json:"TargetDir"`
		Task      string `json:"Task"`
	}
	bodyBytes, _ := json.Marshal(reqBody{TargetDir: t.TempDir(), Task: "test task"})
	req := httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(string(bodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}
}

// TestStatusMethodNotAllowed: POST /status → 405
func TestStatusMethodNotAllowed(t *testing.T) {
	store := session.New(t.TempDir())
	srv := New(store)

	req := httptest.NewRequest(http.MethodPost, "/status", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// TestVerifyMethodNotAllowed: GET /verify → 405
func TestVerifyMethodNotAllowed(t *testing.T) {
	store := session.New(t.TempDir())
	srv := New(store)

	req := httptest.NewRequest(http.MethodGet, "/verify", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// TestResetBudgetMethodNotAllowed: GET /reset-budget → 405
func TestResetBudgetMethodNotAllowed(t *testing.T) {
	store := session.New(t.TempDir())
	srv := New(store)

	req := httptest.NewRequest(http.MethodGet, "/reset-budget", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// TestHistoryEndpointWithData: GET /history sau khi có entries → JSON array không rỗng
func TestHistoryEndpointWithData(t *testing.T) {
	store := session.New(t.TempDir())
	store.UpdateCost("claude-haiku", 100, 50, 0.005)
	srv := New(store)

	req := httptest.NewRequest(http.MethodGet, "/history", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var entries []session.HistoryEntry
	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected non-empty entries array after UpdateCost")
	}
}

// TestVerifyBadJSONBody: POST /verify với body không hợp lệ → 400
func TestVerifyBadJSONBody(t *testing.T) {
	store := session.New(t.TempDir())
	srv := New(store)

	req := httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
