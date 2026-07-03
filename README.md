# Kashy

> Watches your AI spending so you don't have to.

Kashy is a lightweight local proxy that sits between your AI agent and the LLM API. Every token is tracked, every dollar is visible, and budget limits are enforced before the bill arrives.

**Single binary. ~10 MB. ~15 MB RAM idle. No runtime required.**

🇻🇳 [Đọc bằng tiếng Việt](README.vi.md)

---

## The problem

Solo developers using BYOK (Bring Your Own Key) with OpenRouter or any OpenAI-compatible API have no real-time cost visibility. The bill arrives *after* the damage. Kashy intercepts every call and shows the cost as it happens.

---

## Install

**Prerequisites:** [Go 1.21+](https://go.dev/dl/)

```bash
git clone https://github.com/nicodolas/kashy
cd kashy
go build -o kashy ./cmd/kashy/        # Linux / macOS
go build -o kashy.exe ./cmd/kashy/    # Windows
```

**Add to PATH (Windows PowerShell):**
```powershell
$bin = "$env:USERPROFILE\bin"
New-Item -ItemType Directory -Force -Path $bin | Out-Null
Copy-Item kashy.exe "$bin\kashy.exe"
$p = [Environment]::GetEnvironmentVariable("PATH","User")
if ($p -notlike "*$bin*") {
    [Environment]::SetEnvironmentVariable("PATH","$p;$bin","User")
}
# Reopen terminal, then: kashy --version
```

**Add to PATH (Linux / macOS):**
```bash
sudo mv kashy /usr/local/bin/
kashy --version
```

---

## Quick start (< 5 minutes)

### 1 — Set your API key

```bash
kashy config set-key sk-or-...
```

Get a free key at [openrouter.ai/keys](https://openrouter.ai/keys)

### 2 — Start Kashy

```bash
kashy start
# [kashy] proxy started  →  http://localhost:4000/v1
# [kashy] status server  →  http://localhost:4001/status
```

### 3 — Point your agent at Kashy

| Agent | Setting |
|---|---|
| **OMX** (`.codex/config.toml`) | `openai_base_url = "http://localhost:4000/v1"` |
| **OpenCode** (`opencode.json`) | `{ "provider": { "openrouter": { "options": { "baseURL": "http://localhost:4000/v1" } } } }` |
| **Any OpenAI-compatible agent** | Set base URL to `http://localhost:4000/v1` |

Or let Kashy patch everything automatically:

```bash
kashy doctor --fix
```

### 4 — Watch your spending

```bash
kashy status     # live session: tokens, cost, budget bar
kashy history    # spending by day (last 30 days)
kashy balance    # live OpenRouter account balance
```

---

## Commands

| Command | Description |
|---|---|
| `kashy start` | Start proxy (:4000) + status server (:4001) |
| `kashy stop` | Stop the running daemon |
| `kashy status` | Current session spending + ASCII budget bar |
| `kashy history` | Spending grouped by day (last 30 days) |
| `kashy balance` | Live OpenRouter usage: today / week / month |
| `kashy update` | Update Kashy to the latest version |
| `kashy config set-key <key>` | Set OpenRouter API key |
| `kashy config set-budget <usd>` | Set session hard-stop budget (USD) |
| `kashy config show` | Show current configuration |
| `kashy doctor` | Check which agents are connected to Kashy |
| `kashy doctor --fix` | Auto-patch agent config files |
| `kashy mcp` | Start as MCP stdio server |

---

## MCP Integration

Add Kashy as an MCP tool server so your agent can query cost data directly:

```json
{
  "mcpServers": {
    "kashy": {
      "command": "kashy",
      "args": ["mcp"]
    }
  }
}
```

**Available MCP tools:**

| Tool | Description |
|---|---|
| `kashy_cost_status` | Current session cost, tokens, and budget status |
| `kashy_cost_history` | Last N history entries (default 10, max 50) |
| `kashy_verify_done` | Run tests + LLM review; returns AUTO-CHECKED / NEEDS-REVIEW / FAILED / NO-TESTS |
| `kashy_reset_budget` | Reset session cost accumulator to $0.00 |
| `kashy_budget_remaining` | Live OpenRouter spending (today / week / month) |
| `kashy_account_usage` | Full key info: limits, free tier status, rate limits |

---

## Configuration

Config file: `~/.kashy/config.toml`

```toml
[providers.openrouter]
api_key  = "sk-or-..."
base_url = "https://openrouter.ai/api/v1"

[budget]
session_hard_stop = 1.00   # USD — returns HTTP 429 when exceeded
warn_at           = 0.80   # warn at 80% via X-Kashy-Budget-Warning header
daily_limit       = 10.00  # USD — resets at midnight (local time)

[loop]
default_model = "anthropic/claude-3-haiku"
max_iter      = 50
```

**Environment variables** (override config file):

| Variable | Effect |
|---|---|
| `OPENROUTER_API_KEY` | Override `api_key` |
| `KASHY_CONFIG` | Override config file path (useful for testing) |

---

## How it works

```
Your AI Agent
     │
     │  POST /v1/chat/completions
     ▼
┌─────────────────────┐
│   Kashy Proxy :4000 │  ← checks session budget & daily limit
│                     │  ← injects Authorization header
│                     │  ← extracts token usage from response
│                     │  ← records cost to ~/.kashy/history.jsonl
└────────┬────────────┘
         │
         │  forwards request
         ▼
   OpenRouter / LLM API
```

When the session budget is reached, Kashy returns HTTP 429 immediately — the LLM is never called, so you pay nothing extra.

---


## License

MIT — see [LICENSE](LICENSE)
