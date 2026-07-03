// Package httpserver exposes Kashy status and verify API over HTTP (port 4001).
package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/nicodolas/kashy/internal/session"
	"github.com/nicodolas/kashy/internal/verify"
)

// Server is the Kashy HTTP status server.
type Server struct {
	store *session.Store
}

// New creates a Server backed by the given Store.
func New(store *session.Store) *Server {
	return &Server{store: store}
}

// Handler returns an http.Handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/verify", s.handleVerify)
	mux.HandleFunc("/history", s.handleHistory)
	mux.HandleFunc("/reset-budget", s.handleResetBudget)
	return mux
}

// GET /status — returns current session state as JSON.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	st := s.store.ReadState()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(st)
}

// POST /verify — runs verify gate and returns result.
func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var cfg verify.GateConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, fmt.Sprintf("invalid body: %v", err), http.StatusBadRequest)
		return
	}
	result := verify.VerifyDone(cfg)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// GET /history — returns last 50 session history entries.
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	entries, err := s.store.ReadHistory(50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if entries == nil {
		entries = []session.HistoryEntry{}
	}
	json.NewEncoder(w).Encode(entries)
}

// POST /reset-budget — clears session state.
func (s *Server) handleResetBudget(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.store.ClearState(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, `{"ok":true,"message":"Session budget reset."}`)
}

