package provider

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenRouter(t *testing.T) {
	p := OpenRouter("test-key-123")
	if p.Name() != "openrouter" {
		t.Errorf("Name() = %q, want %q", p.Name(), "openrouter")
	}
	if p.BaseURL() != "https://openrouter.ai/api/v1" {
		t.Errorf("BaseURL() = %q, want %q", p.BaseURL(), "https://openrouter.ai/api/v1")
	}
	if p.APIKey() != "test-key-123" {
		t.Errorf("APIKey() = %q, want %q", p.APIKey(), "test-key-123")
	}
}

func TestDirect(t *testing.T) {
	p := Direct("mybackend", "https://api.example.com/v1", "secret")
	if p.Name() != "mybackend" {
		t.Errorf("Name() = %q, want %q", p.Name(), "mybackend")
	}
	if p.BaseURL() != "https://api.example.com/v1" {
		t.Errorf("BaseURL() = %q, want %q", p.BaseURL(), "https://api.example.com/v1")
	}
	if p.APIKey() != "secret" {
		t.Errorf("APIKey() = %q, want %q", p.APIKey(), "secret")
	}
}

func TestGetModelCostNoCacheReturns0(t *testing.T) {
	var cache PricingCache
	cost := cache.GetModelCost("unknown-model/v1", 1000, 500)
	if cost != 0 {
		t.Errorf("expected 0 for unknown model, got %f", cost)
	}
}

// TestFetchPricingHappyPath: mock /models server → FetchPricing() populate cache → GetModelCost() đúng
func TestFetchPricingHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// prompt=0.000001/token, completion=0.000002/token
		w.Write([]byte(`{"data":[{"id":"claude-3-haiku","pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`))
	}))
	defer srv.Close()

	var cache PricingCache
	p := Direct("test", srv.URL, "test-key")
	if err := cache.FetchPricing(p); err != nil {
		t.Fatalf("FetchPricing error: %v", err)
	}

	// 1M prompt tokens * $1/M = $1.0; 500K comp tokens * $2/M = $1.0 → total $2.0
	cost := cache.GetModelCost("claude-3-haiku", 1_000_000, 500_000)
	const want = 2.0
	if cost < want-0.0001 || cost > want+0.0001 {
		t.Errorf("GetModelCost: got %f, want ~%f", cost, want)
	}
}

// TestFetchPricingNetworkError: server không tồn tại → FetchPricing trả nil, cache trống
func TestFetchPricingNetworkError(t *testing.T) {
	var cache PricingCache
	p := Direct("test", "http://127.0.0.1:1", "") // port 1 luôn bị từ chối
	err := cache.FetchPricing(p)
	if err != nil {
		t.Errorf("FetchPricing should return nil on network error (silent fail), got: %v", err)
	}
	cost := cache.GetModelCost("any-model", 1000, 500)
	if cost != 0 {
		t.Errorf("expected 0 for empty cache, got %f", cost)
	}
}

// TestFetchPricingMalformedJSON: server trả broken JSON → FetchPricing trả nil, cache trống
func TestFetchPricingMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{broken json"))
	}))
	defer srv.Close()

	var cache PricingCache
	p := Direct("test", srv.URL, "")
	err := cache.FetchPricing(p)
	if err != nil {
		t.Errorf("FetchPricing should return nil on parse error (silent fail), got: %v", err)
	}
	cost := cache.GetModelCost("any-model", 1000, 500)
	if cost != 0 {
		t.Errorf("expected 0 for empty cache after malformed JSON, got %f", cost)
	}
}

// TestGetModelCostMath: kiểm tra công thức per-million-token
func TestGetModelCostMath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"id":"m","pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`))
	}))
	defer srv.Close()

	var cache PricingCache
	cache.FetchPricing(Direct("test", srv.URL, ""))

	// 100 prompt tokens * (0.000001 * 1M / 1M) = 0.0001
	// 50 comp tokens * (0.000002 * 1M / 1M) = 0.0001
	// total = 0.0002
	cost := cache.GetModelCost("m", 100, 50)
	const want = 0.0002
	if cost < want-0.00001 || cost > want+0.00001 {
		t.Errorf("GetModelCost math: got %f, want ~%f", cost, want)
	}
}
