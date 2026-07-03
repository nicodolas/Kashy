// Package openrouter queries the OpenRouter API directly for account stats.
// This gives Kashy real spending data without needing to intercept proxy calls.
package openrouter

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const baseURL = "https://openrouter.ai/api/v1"

// Credits holds account credit balance from OpenRouter.
// Note: free-tier keys return total_credits=0 — use KeyInfo for usage data instead.
type Credits struct {
	TotalCredits float64 `json:"total_credits"` // credits purchased (0 for free tier)
	TotalUsage   float64 `json:"total_usage"`   // credits used
}

// Remaining returns unused credits. May be negative for free-tier (no credits purchased).
func (c Credits) Remaining() float64 {
	return c.TotalCredits - c.TotalUsage
}

// UsedPercent returns percentage of credits used (0-100). Returns 0 if no credits purchased.
func (c Credits) UsedPercent() float64 {
	if c.TotalCredits <= 0 {
		return 0
	}
	return (c.TotalUsage / c.TotalCredits) * 100
}

// KeyInfo holds detailed usage and limit info for an API key.
// Sourced from GET /api/v1/key — works for all key types including free tier.
type KeyInfo struct {
	Label          string   `json:"label"`
	IsManagementKey bool    `json:"is_management_key"`
	IsFreeTier     bool     `json:"is_free_tier"`

	// Usage (in USD)
	Usage        float64 `json:"usage"`         // all time
	UsageDaily   float64 `json:"usage_daily"`   // today
	UsageWeekly  float64 `json:"usage_weekly"`  // this week
	UsageMonthly float64 `json:"usage_monthly"` // this month

	// Limits (nil = no limit set)
	Limit          *float64 `json:"limit"`
	LimitRemaining *float64 `json:"limit_remaining"`
	LimitReset     *string  `json:"limit_reset"`

	ExpiresAt *string `json:"expires_at"`

	RateLimit struct {
		Requests int    `json:"requests"`
		Interval string `json:"interval"`
	} `json:"rate_limit"`
}

// Summary returns a human-readable spending summary for the key.
func (k KeyInfo) Summary() string {
	freeTierStr := ""
	if k.IsFreeTier {
		freeTierStr = " (free tier)"
	}

	limitStr := "none"
	if k.Limit != nil {
		limitStr = fmt.Sprintf("$%.4f", *k.Limit)
	}
	remainingStr := "n/a"
	if k.LimitRemaining != nil {
		remainingStr = fmt.Sprintf("$%.4f", *k.LimitRemaining)
	}

	return fmt.Sprintf(
		"OpenRouter Key: %s%s\n"+
			"─────────────────────────────\n"+
			"Usage today:   $%.6f\n"+
			"Usage week:    $%.6f\n"+
			"Usage month:   $%.6f\n"+
			"Usage total:   $%.6f\n"+
			"Key limit:     %s\n"+
			"Remaining:     %s\n",
		k.Label, freeTierStr,
		k.UsageDaily,
		k.UsageWeekly,
		k.UsageMonthly,
		k.Usage,
		limitStr,
		remainingStr,
	)
}

// Client queries the OpenRouter API.
type Client struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// New creates a Client with the given API key.
func New(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    baseURL,
	}
}

// GetCredits fetches total credits purchased and used.
// Note: Returns total_credits=0 for free-tier keys. Use GetKeyInfo for usage data.
func (c *Client) GetCredits() (Credits, error) {
	var resp struct {
		Data Credits `json:"data"`
	}
	if err := c.get("/credits", &resp); err != nil {
		return Credits{}, err
	}
	return resp.Data, nil
}

// GetKeyInfo fetches detailed usage and limit info for this API key.
// Works for all key types including free tier.
func (c *Client) GetKeyInfo() (KeyInfo, error) {
	var resp struct {
		Data KeyInfo `json:"data"`
	}
	if err := c.get("/key", &resp); err != nil {
		return KeyInfo{}, err
	}
	return resp.Data, nil
}

// get makes a GET request to the given path and decodes JSON into v.
func (c *Client) get(path string, v any) error {
	base := c.baseURL
	if base == "" {
		base = baseURL
	}
	req, err := http.NewRequest("GET", base+path, nil)
	if err != nil {
		return fmt.Errorf("openrouter: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("openrouter: request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("openrouter: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		n := len(body)
		if n > 150 {
			n = 150
		}
		return fmt.Errorf("openrouter: status %d: %s", resp.StatusCode, string(body[:n]))
	}

	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("openrouter: parse response: %w", err)
	}
	return nil
}
