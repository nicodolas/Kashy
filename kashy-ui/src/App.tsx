import { useEffect, useState, useCallback, useRef } from "react";
import {
    fetchStatus,
    fetchHistory,
    resetBudget,
    formatUSD,
    formatDate,
    SessionState,
    HistoryEntry,
} from "./api";
import { Command } from "@tauri-apps/plugin-shell";

const REFRESH_MS = 3000;
const DEFAULT_BUDGET = 1.0;

// Spawn kashy sidecar with given args, returns the Child process (or null on error)
async function spawnKashy(args: string[]) {
    try {
        // Production: use bundled sidecar (relative to src-tauri/)
        const cmd = Command.sidecar("binaries/kashy", args);
        return await cmd.spawn();
    } catch {
        // Dev fallback: use kashy from PATH
        try {
            const cmd = Command.create("kashy", args);
            return await cmd.spawn();
        } catch {
            return null;
        }
    }
}

function BudgetBar({ used, limit }: { used: number; limit: number }) {
    const pct = limit > 0 ? Math.min((used / limit) * 100, 100) : 0;
    const cls = pct >= 90 ? "danger" : pct >= 75 ? "warn" : "";
    return (
        <div className="budget-bar-wrap">
            <div className="budget-bar-header">
                <span>Session Budget</span>
                <span>
                    {formatUSD(used)} / ${limit.toFixed(2)} ({pct.toFixed(1)}%)
                </span>
            </div>
            <div className="budget-bar-track">
                <div
                    className={`budget-bar-fill ${cls}`}
                    style={{ width: `${pct}%` }}
                />
            </div>
        </div>
    );
}

export default function App() {
    const [state, setState] = useState<SessionState | null>(null);
    const [history, setHistory] = useState<HistoryEntry[]>([]);
    const [proxyRunning, setProxyRunning] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [starting, setStarting] = useState(false);
    const proxyChild = useRef<Awaited<ReturnType<typeof spawnKashy>>>(null);
    const autoStarted = useRef(false);

    const load = useCallback(async () => {
        try {
            const [s, h] = await Promise.all([fetchStatus(), fetchHistory()]);
            setState(s);
            setHistory(h ?? []);
            setProxyRunning(true);
            setError(null);
        } catch {
            setProxyRunning(false);
        }
    }, []);

    // Auto-start proxy on app open (once)
    useEffect(() => {
        if (autoStarted.current) return;
        autoStarted.current = true;

        async function tryAutoStart() {
            // First check if already running
            try {
                await fetchStatus();
                setProxyRunning(true);
                return;
            } catch {
                // Not running — start it
            }

            setStarting(true);
            const child = await spawnKashy(["start"]);
            proxyChild.current = child;

            // Poll until proxy responds (max 8s)
            for (let i = 0; i < 16; i++) {
                await new Promise((r) => setTimeout(r, 500));
                try {
                    await fetchStatus();
                    setProxyRunning(true);
                    setError(null);
                    setStarting(false);
                    return;
                } catch {
                    /* still starting */
                }
            }
            setStarting(false);
            setError("Proxy failed to start. Check that kashy is installed.");
        }

        tryAutoStart();
    }, []);

    // Periodic refresh
    useEffect(() => {
        load();
        const t = setInterval(load, REFRESH_MS);
        return () => clearInterval(t);
    }, [load]);

    // Stop proxy on app close
    useEffect(() => {
        return () => {
            proxyChild.current?.kill().catch(() => { });
        };
    }, []);

    async function handleStopProxy() {
        try {
            await spawnKashy(["stop"]);
        } catch {
            /* ignore */
        }
        proxyChild.current = null;
        setTimeout(load, 800);
    }

    async function handleStartProxy() {
        setStarting(true);
        const child = await spawnKashy(["start"]);
        proxyChild.current = child;
        for (let i = 0; i < 12; i++) {
            await new Promise((r) => setTimeout(r, 500));
            try {
                await fetchStatus();
                setProxyRunning(true);
                setStarting(false);
                return;
            } catch {
                /* still starting */
            }
        }
        setStarting(false);
    }

    async function handleResetBudget() {
        await resetBudget();
        await load();
    }

    const costColor =
        !state || state.total_cost_usd === 0
            ? ""
            : state.total_cost_usd >= DEFAULT_BUDGET * 0.9
                ? "red"
                : state.total_cost_usd >= DEFAULT_BUDGET * 0.75
                    ? "yellow"
                    : "green";

    return (
        <div className="app">
            {/* Header */}
            <div className="header">
                <h1>
                    <span>kashy</span> · AI Spending Monitor
                </h1>
                <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                    <span className={`status-dot ${proxyRunning ? "running" : ""}`} />
                    <span style={{ fontSize: 13, color: "#94a3b8" }}>
                        {starting
                            ? "Starting proxy..."
                            : proxyRunning
                                ? "Proxy running"
                                : "Proxy stopped"}
                    </span>
                </div>
            </div>

            {/* Error banner */}
            {error && (
                <div
                    style={{
                        background: "#1e1215",
                        border: "1px solid #7f1d1d",
                        borderRadius: 8,
                        padding: "12px 16px",
                        marginBottom: 24,
                        fontSize: 13,
                        color: "#fca5a5",
                    }}
                >
                    {error}
                </div>
            )}

            {/* Starting indicator */}
            {starting && (
                <div
                    style={{
                        background: "#1a1f2e",
                        border: "1px solid #2d3148",
                        borderRadius: 8,
                        padding: "12px 16px",
                        marginBottom: 24,
                        fontSize: 13,
                        color: "#818cf8",
                    }}
                >
                    ⏳ Starting Kashy proxy on :4000…
                </div>
            )}

            {/* Stats grid */}
            <div className="grid">
                <div className="card">
                    <div className="card-label">Session Cost</div>
                    <div className={`card-value ${costColor}`}>
                        {state ? formatUSD(state.total_cost_usd) : "—"}
                    </div>
                </div>
                <div className="card">
                    <div className="card-label">LLM Calls</div>
                    <div className="card-value">{state?.call_count ?? "—"}</div>
                </div>
                <div className="card">
                    <div className="card-label">Prompt Tokens</div>
                    <div className="card-value">
                        {state ? state.prompt_tokens.toLocaleString() : "—"}
                    </div>
                </div>
                <div className="card">
                    <div className="card-label">Completion Tokens</div>
                    <div className="card-value">
                        {state ? state.completion_tokens.toLocaleString() : "—"}
                    </div>
                </div>
            </div>

            {/* Budget bar */}
            <BudgetBar used={state?.total_cost_usd ?? 0} limit={DEFAULT_BUDGET} />

            {/* History */}
            <div className="section-title">Recent Calls</div>
            <div className="history-table">
                {history.length === 0 ? (
                    <div className="empty-state">
                        No calls yet — point your AI agent to{" "}
                        <code style={{ color: "#818cf8" }}>http://localhost:4000/v1</code>
                    </div>
                ) : (
                    <table>
                        <thead>
                            <tr>
                                <th>Time</th>
                                <th>Model</th>
                                <th>Prompt</th>
                                <th>Completion</th>
                                <th>Cost</th>
                            </tr>
                        </thead>
                        <tbody>
                            {[...history].reverse().slice(0, 20).map((e, i) => (
                                <tr key={i}>
                                    <td>{formatDate(e.timestamp)}</td>
                                    <td style={{ color: "#818cf8" }}>{e.model || "—"}</td>
                                    <td>{e.prompt_tokens.toLocaleString()}</td>
                                    <td>{e.completion_tokens.toLocaleString()}</td>
                                    <td style={{ color: "#4ade80" }}>{formatUSD(e.cost_usd)}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                )}
            </div>

            {/* Actions */}
            <div className="actions">
                {proxyRunning ? (
                    <button className="btn btn-danger" onClick={handleStopProxy}>
                        Stop Proxy
                    </button>
                ) : (
                    <button
                        className="btn btn-primary"
                        onClick={handleStartProxy}
                        disabled={starting}
                    >
                        {starting ? "Starting…" : "Start Proxy"}
                    </button>
                )}
                <button className="btn btn-secondary" onClick={handleResetBudget}>
                    Reset Budget
                </button>
                <button className="btn btn-secondary" onClick={load}>
                    Refresh
                </button>
            </div>

            {/* Footer */}
            {state?.last_model && (
                <div style={{ marginTop: 16, fontSize: 12, color: "#475569" }}>
                    Last model: {state.last_model} · Updated:{" "}
                    {formatDate(state.last_updated)}
                </div>
            )}
        </div>
    );
}
