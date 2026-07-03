// Package proxy implements the Kashy cost-metering HTTP proxy.
// It forwards requests to an upstream LLM provider (OpenRouter or any
// OpenAI-compatible endpoint), extracts token usage from each response,
// accumulates session cost, and enforces configurable budget limits.
package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/nicodolas/kashy/internal/provider"
	"github.com/nicodolas/kashy/internal/session"
)

// Config holds proxy configuration.
type Config struct {
	Provider        provider.Provider
	Store           *session.Store
	Pricing         *provider.PricingCache
	SessionHardStop float64 // USD; 0 = unlimited
	WarnAt          float64 // fraction 0-1 (e.g. 0.80)
	DailyLimit      float64 // USD; 0 = unlimited — checked against today's accumulated cost
}

// UsageEvent is emitted after each LLM call.
type UsageEvent struct {
	Model        string
	PromptTok    int
	CompTok      int
	CostUSD      float64
	ResponseMS   int64
}

// Proxy is the HTTP proxy server.
type Proxy struct {
	cfg     Config
	mu      sync.Mutex
	usageCb func(UsageEvent)
	client  *http.Client // reused across requests for connection pooling
}

// New creates a new Proxy.
func New(cfg Config) *Proxy {
	return &Proxy{
		cfg:    cfg,
		client: &http.Client{Timeout: 300 * time.Second},
	}
}

// SetUsageCallback sets a callback invoked after each LLM call with usage data.
func (p *Proxy) SetUsageCallback(fn func(UsageEvent)) {
	p.mu.Lock()
	p.usageCb = fn
	p.mu.Unlock()
}

// Handler returns an http.Handler suitable for use with http.ListenAndServe.
func (p *Proxy) Handler() http.Handler {
	return http.HandlerFunc(p.serveHTTP)
}

func (p *Proxy) serveHTTP(w http.ResponseWriter, r *http.Request) {
	// Check session budget before forwarding
	if p.cfg.SessionHardStop > 0 {
		st := p.cfg.Store.ReadState()
		if st.TotalCostUSD >= p.cfg.SessionHardStop {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			msg := fmt.Sprintf(
				`{"error":"session_budget_reached","used":%.6f,"limit":%.6f,"message":"Run 'kashy config set-budget' to increase your limit or wait for next session."}`,
				st.TotalCostUSD, p.cfg.SessionHardStop,
			)
			io.WriteString(w, msg)
			return
		}
	}

	// Check daily limit before forwarding
	if p.cfg.DailyLimit > 0 {
		dailyCost := p.cfg.Store.TodayCostUSD()
		if dailyCost >= p.cfg.DailyLimit {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			msg := fmt.Sprintf(
				`{"error":"daily_limit_reached","used_today":%.6f,"daily_limit":%.6f,"message":"Daily budget exceeded. Resets at midnight."}`,
				dailyCost, p.cfg.DailyLimit,
			)
			io.WriteString(w, msg)
			return
		}
	}

	// Build upstream URL
	upstreamBase := strings.TrimRight(p.cfg.Provider.BaseURL(), "/")
	upstreamURL := upstreamBase + r.URL.Path
	if r.URL.RawQuery != "" {
		upstreamURL += "?" + r.URL.RawQuery
	}
	parsed, err := url.Parse(upstreamURL)
	if err != nil {
		http.Error(w, "invalid upstream URL", http.StatusBadGateway)
		return
	}

	// Buffer request body so we can detect streaming intent
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadGateway)
		return
	}
	r.Body.Close()

	// Detect streaming
	isStream := false
	var bodyMap map[string]interface{}
	if json.Unmarshal(body, &bodyMap) == nil {
		if v, ok := bodyMap["stream"]; ok {
			if b, ok := v.(bool); ok {
				isStream = b
			}
		}
	}

	// Build upstream request
	upReq, err := http.NewRequestWithContext(r.Context(), r.Method, parsed.String(), bytes.NewReader(body))
	if err != nil {
		http.Error(w, "build upstream request", http.StatusBadGateway)
		return
	}
	// Copy headers — strip Authorization from client to prevent key leakage.
	// Kashy injects its own key below; the client's key must never reach upstream.
	for k, vs := range r.Header {
		if strings.EqualFold(k, "Authorization") {
			continue // always stripped — injected below
		}
		for _, v := range vs {
			upReq.Header.Add(k, v)
		}
	}
	upReq.Header.Set("Host", parsed.Host)
	// Inject auth
	if key := p.cfg.Provider.APIKey(); key != "" {
		upReq.Header.Set("Authorization", "Bearer "+key)
	}

	start := time.Now()
	upResp, err := p.client.Do(upReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("upstream error: %v", err), http.StatusBadGateway)
		return
	}
	defer upResp.Body.Close()

	// Copy response headers
	for k, vs := range upResp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}

	// Add warn header if approaching budget
	if p.cfg.SessionHardStop > 0 && p.cfg.WarnAt > 0 {
		st := p.cfg.Store.ReadState()
		fraction := st.TotalCostUSD / p.cfg.SessionHardStop
		if fraction >= p.cfg.WarnAt {
			w.Header().Set("X-Kashy-Budget-Warning",
				fmt.Sprintf("%.1f%% of $%.4f session budget used", fraction*100, p.cfg.SessionHardStop))
		}
	}

	w.WriteHeader(upResp.StatusCode)

	if isStream {
		p.pipeStream(w, upResp, start, bodyMap)
	} else {
		p.pipeJSON(w, upResp, start, bodyMap)
	}
}

// pipeJSON buffers the full JSON response, extracts usage, then writes to client.
func (p *Proxy) pipeJSON(w http.ResponseWriter, resp *http.Response, start time.Time, reqBody map[string]interface{}) {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	w.Write(respBody)

	// Extract usage from response
	var result map[string]interface{}
	if json.Unmarshal(respBody, &result) == nil {
		p.handleUsage(result, reqBody, time.Since(start).Milliseconds())
	}
}

// pipeStream passes SSE chunks to the client immediately and extracts usage from final chunks.
// Uses bufio.Scanner on a tee'd copy to read line-by-line, preventing the split-chunk bug.
//
// Architecture: resp.Body is tee'd into an in-memory buffer while simultaneously
// being forwarded to the client. After all bytes arrive, the buffer is scanned
// for the last "data: ..." SSE line to extract usage. This avoids the pipe-based
// deadlock where Scanner.Scan() → TeeReader.Read() → pw.Write() could block
// if the client-write goroutine was also blocked on w.Write().
func (p *Proxy) pipeStream(w http.ResponseWriter, resp *http.Response, start time.Time, reqBody map[string]interface{}) {
	flusher, canFlush := w.(http.Flusher)

	// Read all SSE bytes, forwarding to client as we go.
	// bufio.Scanner needs the full stream; we collect it in a bytes.Buffer via TeeReader,
	// but we use a channel to decouple writing to client from reading for scanning.
	var sseCapture bytes.Buffer
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			w.Write(chunk)
			if canFlush {
				flusher.Flush()
			}
			sseCapture.Write(chunk)
		}
		if err != nil {
			break
		}
	}

	// Now scan the captured bytes line-by-line for the last data: ... SSE line.
	// Scanner handles \r\n and split lines correctly since we have the full stream.
	var lastDataLine string
	scanner := bufio.NewScanner(&sseCapture)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") && line != "data: [DONE]" {
			lastDataLine = strings.TrimPrefix(line, "data: ")
		}
	}

	if lastDataLine != "" {
		var chunk map[string]interface{}
		if json.Unmarshal([]byte(lastDataLine), &chunk) == nil {
			p.handleUsage(chunk, reqBody, time.Since(start).Milliseconds())
		}
	}
}

// handleUsage extracts token usage from an LLM response and records cost.
func (p *Proxy) handleUsage(result, reqBody map[string]interface{}, ms int64) {
	usage, _ := result["usage"].(map[string]interface{})
	if usage == nil {
		return
	}

	promptTok := int(toFloat(usage["prompt_tokens"]))
	compTok := int(toFloat(usage["completion_tokens"]))

	// Model: from response first, then request
	model := ""
	if m, ok := result["model"].(string); ok {
		model = m
	}
	if model == "" {
		if m, ok := reqBody["model"].(string); ok {
			model = m
		}
	}

	costUSD := 0.0
	if p.cfg.Pricing != nil {
		costUSD = p.cfg.Pricing.GetModelCost(model, promptTok, compTok)
	}

	// Persist
	if p.cfg.Store != nil {
		_ = p.cfg.Store.UpdateCost(model, promptTok, compTok, costUSD)
	}

	// Callback
	p.mu.Lock()
	cb := p.usageCb
	p.mu.Unlock()
	if cb != nil {
		cb(UsageEvent{
			Model:      model,
			PromptTok:  promptTok,
			CompTok:    compTok,
			CostUSD:    costUSD,
			ResponseMS: ms,
		})
	}
}

func toFloat(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	}
	return 0
}

