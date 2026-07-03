package openrouter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestClient tạo Client trỏ vào httptest.Server.
func newTestClient(srv *httptest.Server, apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		httpClient: srv.Client(),
		baseURL:    srv.URL,
	}
}

// TestClientGetCreditsViaClientStruct: dùng Client thật (không bypass client.get())
func TestClientGetCreditsViaClientStruct(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/credits" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing/wrong auth header: %q", r.Header.Get("Authorization"))
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"total_credits": 100.0,
				"total_usage":   25.75,
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv, "test-key")
	credits, err := c.GetCredits()
	if err != nil {
		t.Fatalf("GetCredits() error: %v", err)
	}
	if credits.TotalCredits != 100.0 {
		t.Errorf("TotalCredits: got %f, want 100.0", credits.TotalCredits)
	}
	if credits.TotalUsage != 25.75 {
		t.Errorf("TotalUsage: got %f, want 25.75", credits.TotalUsage)
	}
}

// TestClientGetKeyInfoViaClientStruct: dùng Client thật với GetKeyInfo()
func TestClientGetKeyInfoViaClientStruct(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/key" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"label":       "my-key",
				"usage_daily": 0.5,
				"usage":       10.0,
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv, "test-key")
	info, err := c.GetKeyInfo()
	if err != nil {
		t.Fatalf("GetKeyInfo() error: %v", err)
	}
	if info.Label != "my-key" {
		t.Errorf("Label: got %q, want %q", info.Label, "my-key")
	}
	if info.UsageDaily != 0.5 {
		t.Errorf("UsageDaily: got %f, want 0.5", info.UsageDaily)
	}
}

// TestClientGetHTTP401: mock trả 401 → error chứa "401"
func TestClientGetHTTP401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := newTestClient(srv, "bad-key")
	_, err := c.GetCredits()
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should contain '401', got: %v", err)
	}
}

// TestClientGetHTTP500: mock trả 500 → error chứa "500"
func TestClientGetHTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(srv, "key")
	_, err := c.GetCredits()
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should contain '500', got: %v", err)
	}
}

// TestClientGetMalformedJSON: mock trả 200 nhưng body không phải JSON
func TestClientGetMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{broken json"))
	}))
	defer srv.Close()

	c := newTestClient(srv, "key")
	_, err := c.GetCredits()
	if err == nil {
		t.Fatal("expected parse error for malformed JSON, got nil")
	}
}

// TestKeyInfoSummary: Summary() chứa label, usage_daily, limit
func TestKeyInfoSummary(t *testing.T) {
	daily := 0.005
	_ = daily
	info := KeyInfo{
		Label:      "prod-key",
		UsageDaily: 0.005,
		UsageWeekly: 0.02,
		UsageMonthly: 0.1,
		Usage:      5.0,
	}
	out := info.Summary()
	if !strings.Contains(out, "prod-key") {
		t.Errorf("Summary() missing label, got: %s", out)
	}
	if !strings.Contains(out, "$0.005000") {
		t.Errorf("Summary() missing daily usage $0.005000, got: %s", out)
	}
	if !strings.Contains(out, "$5.000000") {
		t.Errorf("Summary() missing total usage $5.000000, got: %s", out)
	}
}

// TestCreditsUsedPercentNonZero: TotalCredits=200, TotalUsage=50 → 25.0%
func TestCreditsUsedPercentNonZero(t *testing.T) {
	c := Credits{TotalCredits: 200, TotalUsage: 50}
	got := c.UsedPercent()
	if got != 25.0 {
		t.Errorf("UsedPercent: got %f, want 25.0", got)
	}
}

// --- Giữ lại tests cũ ---

func TestCreditsRemaining(t *testing.T) {
	c := Credits{TotalCredits: 50.0, TotalUsage: 30.0}
	if c.Remaining() != 20.0 {
		t.Errorf("expected 20, got %f", c.Remaining())
	}
}

func TestCreditsUsedPercent(t *testing.T) {
	c := Credits{TotalCredits: 0}
	if c.UsedPercent() != 0 {
		t.Error("expected 0 for zero credits")
	}
}
