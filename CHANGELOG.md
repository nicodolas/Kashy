# Changelog

All notable changes to **Kashy** are documented here.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html)

---

## [Unreleased]

---

## [1.1.0] — 2026-07-03

### Added
- **`kashy update`** — self-update command: checks GitHub Releases and replaces binary in-place
- **Update check on start** — `kashy start` checks for newer version (3s timeout, non-blocking on error) and prints notice
- **`kashy stop`** — now actually stops the running daemon via pidfile (previously just printed instructions)
- **`internal/daemon`** — pidfile management package for process lifecycle
- **`internal/updater`** — update checker and self-update package (11 tests)
- **`internal/mcpserver` tests** — 9 new tests via `buildServer()` extracted for testability

### Fixed
- **SSE streaming deadlock** — `pipeStream` replaced `io.Pipe` + `TeeReader` + goroutine with simple sequential read-and-capture, eliminating potential deadlock under slow clients
- **Authorization header leak** — proxy now strips client `Authorization` header before forwarding; only Kashy's own API key reaches upstream
- **`json.Marshal` error ignored** — `verify.LLMReview` now returns error if marshal fails
- **`TodayCostUSD` undercount** — increased history read limit from 500 to 10 000 entries for accurate daily cost enforcement
- **HTTP client per LLM call** — `verify.LLMReview` now uses a shared singleton client instead of creating a new one per call

### Changed
- **`cmd/kashy/main.go` refactored** — split into `cmd_start.go`, `cmd_status.go`, `cmd_balance.go`, `cmd_config.go`, `cmd_doctor.go`, `cmd_mcp.go`, `cmd_update.go`
- **Test suite** — 111 tests across all packages

---

## [1.0.0] — 2026-07-02

### Added
- **Cost meter proxy** on `:4000` — intercepts every LLM call, extracts token usage, accumulates session cost in real time
- **Budget enforcement** — session hard-stop (HTTP 429) + configurable warn threshold header (`X-Kashy-Budget-Warning`)
- **Daily limit** — per-day spending cap enforced via local history scan
- **`kashy start`** — launches proxy + HTTP status server on `:4001` with graceful shutdown
- **`kashy status`** — ASCII budget bar + session token/cost breakdown
- **`kashy history`** — spending grouped by day for the last 30 days
- **`kashy balance`** — live OpenRouter spending (today / week / month) via direct API query
- **`kashy config`** — `set-key`, `set-budget`, `show` subcommands; config at `~/.kashy/config.toml`
- **`kashy doctor`** — scans OMX, OpenCode, and Kiro/Claude Code configs; `--fix` auto-patches base URLs
- **`kashy mcp`** — MCP stdio server with tools: `kashy_cost_status`, `kashy_cost_history`, `kashy_verify_done`, `kashy_reset_budget`, `kashy_budget_remaining`, `kashy_account_usage`
- **Pricing cache** — fetches per-model cost from OpenRouter `/models` on startup; silent degradation if unavailable
- **History trimming** — auto-trims `history.jsonl` to 1 000 entries to prevent unbounded growth
- **Auto-migration** — copies API key from legacy `~/.nico/config.toml` on first run
- **Test suite** — 60+ tests across all packages; `go test ./...` passes on Windows and Linux

---

## Release Guide

```
# 1. Decide version bump (SemVer)
# 2. Update Major/Minor/Patch in internal/version/version.go
# 3. Run: make release V=1.1.0
# 4. Move [Unreleased] section to [1.1.0] in this file
# 5. git add -A && git commit -m "chore: release v1.1.0"
# 6. git tag v1.1.0 && git push origin main --tags
```
