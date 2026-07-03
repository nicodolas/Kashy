package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nicodolas/kashy/internal/provider"
	"github.com/nicodolas/kashy/internal/session"
)

// fakeUpstream returns a test server that responds with a fixed JSON body.
func fakeUpstream(t *testing.T, body string, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write([]byte(body))
	}))
}

func TestProxyForwardsRequest(t *testing.T) {
	upstream := fakeUpstream(t, `{"id":"chatcmpl-1","choices":[]}`, 200)
	defer upstream.Close()

	store := session.New(t.TempDir())
	p := New(Config{
		Provider: provider.Direct("test", upstream.URL, ""),
		Store:    store,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	p.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}
}

func TestProxyExtractsUsage(t *testing.T) {
	respBody := `{
		"id":"chatcmpl-1",
		"model":"claude-3-haiku",
		"usage":{"prompt_tokens":100,"completion_tokens":50}
	}`
	upstream := fakeUpstream(t, respBody, 200)
	defer upstream.Close()

	store := session.New(t.TempDir())
	var events []UsageEvent
	p := New(Config{
		Provider: provider.Direct("test", upstream.URL, ""),
		Store:    store,
	})
	p.SetUsageCallback(func(e UsageEvent) {
		events = append(events, e)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"claude-3-haiku","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	p.Handler().ServeHTTP(w, req)

	if len(events) != 1 {
		t.Fatalf("expected 1 usage event, got %d", len(events))
	}
	if events[0].PromptTok != 100 {
		t.Errorf("PromptTok: got %d, want 100", events[0].PromptTok)
	}
	if events[0].CompTok != 50 {
		t.Errorf("CompTok: got %d, want 50", events[0].CompTok)
	}
}

func TestProxyBudgetStop(t *testing.T) {
	upstream := fakeUpstream(t, `{"id":"1"}`, 200)
	defer upstream.Close()

	store := session.New(t.TempDir())
	// Pre-load session over budget
	store.UpdateCost("model", 0, 0, 5.00) // cost = $5.00

	p := New(Config{
		Provider:        provider.Direct("test", upstream.URL, ""),
		Store:           store,
		SessionHardStop: 1.00, // limit = $1.00
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	p.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
	var errResp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Errorf("error response is not valid JSON: %v", err)
	}
	if errResp["error"] != "session_budget_reached" {
		t.Errorf("wrong error key: %v", errResp["error"])
	}
}

func TestProxyWarnHeader(t *testing.T) {
	upstream := fakeUpstream(t, `{"id":"1","usage":{"prompt_tokens":10,"completion_tokens":5}}`, 200)
	defer upstream.Close()

	store := session.New(t.TempDir())
	// Pre-load session at 85% of budget
	store.UpdateCost("model", 0, 0, 0.85)

	p := New(Config{
		Provider:        provider.Direct("test", upstream.URL, ""),
		Store:           store,
		SessionHardStop: 1.00,
		WarnAt:          0.80,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"m","messages":[]}`))
	w := httptest.NewRecorder()
	p.Handler().ServeHTTP(w, req)

	warn := w.Header().Get("X-Kashy-Budget-Warning")
	if warn == "" {
		t.Error("expected X-Kashy-Budget-Warning header, got none")
	}
}

func TestProxyDailyLimitStop(t *testing.T) {
	upstream := fakeUpstream(t, `{"id":"1"}`, 200)
	defer upstream.Close()

	store := session.New(t.TempDir())
	// Pre-load today's cost over daily limit
	store.UpdateCost("model", 0, 0, 5.00)

	p := New(Config{
		Provider:   provider.Direct("test", upstream.URL, ""),
		Store:      store,
		DailyLimit: 1.00,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	p.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 for daily limit, got %d", w.Code)
	}
	var errResp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Errorf("error response not valid JSON: %v", err)
	}
	if errResp["error"] != "daily_limit_reached" {
		t.Errorf("wrong error key: %v", errResp["error"])
	}
}


func TestProxyStreamingExtractsUsage(t *testing.T) {
	// Fake SSE upstream server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprintf(w, "data: {\"id\":\"1\",\"choices\":[]}\n\n")
		fmt.Fprintf(w, "data: {\"id\":\"1\",\"model\":\"claude-3-haiku\",\"usage\":{\"prompt_tokens\":80,\"completion_tokens\":40}}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()

	store := session.New(t.TempDir())
	var events []UsageEvent
	p := New(Config{
		Provider: provider.Direct("test", upstream.URL, ""),
		Store:    store,
	})
	p.SetUsageCallback(func(e UsageEvent) {
		events = append(events, e)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"claude-3-haiku","messages":[],"stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	p.Handler().ServeHTTP(w, req)

	if len(events) != 1 {
		t.Fatalf("expected 1 usage event, got %d", len(events))
	}
	if events[0].PromptTok != 80 {
		t.Errorf("PromptTok: got %d, want 80", events[0].PromptTok)
	}
	if events[0].CompTok != 40 {
		t.Errorf("CompTok: got %d, want 40", events[0].CompTok)
	}
}

func TestProxyInjectsAuthHeader(t *testing.T) {
	var receivedAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"1","choices":[]}`))
	}))
	defer upstream.Close()

	store := session.New(t.TempDir())
	p := New(Config{
		Provider: provider.Direct("test", upstream.URL, "test-secret-key"),
		Store:    store,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	p.Handler().ServeHTTP(w, req)

	if receivedAuth != "Bearer test-secret-key" {
		t.Errorf("Authorization header: got %q, want %q", receivedAuth, "Bearer test-secret-key")
	}
}

func TestProxyUpstreamError502(t *testing.T) {
	store := session.New(t.TempDir())
	// Port 1 is always refused
	p := New(Config{
		Provider: provider.Direct("test", "http://127.0.0.1:1", ""),
		Store:    store,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	p.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", w.Code)
	}
}

func TestProxyWithPricingCost(t *testing.T) {
	// Mock pricing server
	pricingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		// prompt=0.000001 per-token, completion=0.000002 per-token
		w.Write([]byte(`{"data":[{"id":"claude-3-haiku","pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`))
	}))
	defer pricingServer.Close()

	cache := &provider.PricingCache{}
	pricingProvider := provider.Direct("test", pricingServer.URL, "")
	if err := cache.FetchPricing(pricingProvider); err != nil {
		t.Fatalf("FetchPricing error: %v", err)
	}

	// Upstream returns large usage
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"1","model":"claude-3-haiku","usage":{"prompt_tokens":1000000,"completion_tokens":500000}}`))
	}))
	defer upstream.Close()

	store := session.New(t.TempDir())
	var events []UsageEvent
	p := New(Config{
		Provider: provider.Direct("test", upstream.URL, ""),
		Store:    store,
		Pricing:  cache,
	})
	p.SetUsageCallback(func(e UsageEvent) {
		events = append(events, e)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"claude-3-haiku","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	p.Handler().ServeHTTP(w, req)

	if len(events) != 1 {
		t.Fatalf("expected 1 usage event, got %d", len(events))
	}
	// prompt=1M tokens * $1/M = $1.0; completion=500K tokens * $2/M = $1.0; total = $2.0
	if events[0].CostUSD <= 0 {
		t.Errorf("CostUSD: got %f, want > 0", events[0].CostUSD)
	}
	const wantCost = 2.0
	const tolerance = 0.0001
	if diff := events[0].CostUSD - wantCost; diff > tolerance || diff < -tolerance {
		t.Errorf("CostUSD: got %f, want ~%f", events[0].CostUSD, wantCost)
	}
}
