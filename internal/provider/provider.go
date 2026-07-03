package provider

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Provider abstracts an LLM API backend.
type Provider interface {
	Name() string
	BaseURL() string
	APIKey() string
}

// --- concrete implementation ---

type providerImpl struct {
	name    string
	baseURL string
	apiKey  string
}

func (p *providerImpl) Name() string    { return p.name }
func (p *providerImpl) BaseURL() string { return p.baseURL }
func (p *providerImpl) APIKey() string  { return p.apiKey }

// OpenRouter returns a Provider pre-configured for openrouter.ai.
func OpenRouter(apiKey string) Provider {
	return &providerImpl{
		name:    "openrouter",
		baseURL: "https://openrouter.ai/api/v1",
		apiKey:  apiKey,
	}
}

// Direct returns a Provider with fully-custom name, base URL, and API key.
func Direct(name, baseURL, apiKey string) Provider {
	return &providerImpl{
		name:    name,
		baseURL: baseURL,
		apiKey:  apiKey,
	}
}

// --- pricing cache ---

// ModelInfo holds cost data for a single model.
type ModelInfo struct {
	ID                     string
	PromptCostPerMToken    float64
	CompletionCostPerMToken float64
}

// PricingCache is a thread-safe store for model pricing fetched from a provider.
type PricingCache struct {
	mu      sync.RWMutex
	models  map[string]ModelInfo
	fetched bool
}

// raw JSON shapes used only for parsing
type modelsResponse struct {
	Data []modelEntry `json:"data"`
}

type modelEntry struct {
	ID      string        `json:"id"`
	Pricing modelPricing  `json:"pricing"`
}

type modelPricing struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
}

// FetchPricing calls GET {base_url}/models and populates the cache.
// On any network or parse error it returns nil (silent degradation).
func (c *PricingCache) FetchPricing(p Provider) error {
	url := p.BaseURL() + "/models"

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil // silent
	}
	if key := p.APIKey(); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil // silent
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil // silent
	}

	var raw modelsResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil // silent
	}

	parsed := make(map[string]ModelInfo, len(raw.Data))
	for _, entry := range raw.Data {
		prompt, _ := strconv.ParseFloat(entry.Pricing.Prompt, 64)
		completion, _ := strconv.ParseFloat(entry.Pricing.Completion, 64)
		// prices from OpenRouter are per-token; convert to per-million-token
		parsed[entry.ID] = ModelInfo{
			ID:                     entry.ID,
			PromptCostPerMToken:    prompt * 1_000_000,
			CompletionCostPerMToken: completion * 1_000_000,
		}
	}

	c.mu.Lock()
	c.models = parsed
	c.fetched = true
	c.mu.Unlock()

	return nil
}

// GetModelCost returns the estimated cost in USD for a request.
// Returns 0 if the model is not in the cache.
func (c *PricingCache) GetModelCost(modelID string, promptTok, compTok int) float64 {
	c.mu.RLock()
	info, ok := c.models[modelID]
	c.mu.RUnlock()
	if !ok {
		return 0
	}
	return info.PromptCostPerMToken*float64(promptTok)/1_000_000 +
		info.CompletionCostPerMToken*float64(compTok)/1_000_000
}
