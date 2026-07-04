// api.ts — thin wrapper around Kashy's HTTP status server (:4001)

const BASE = "http://localhost:4001";

export interface SessionState {
    session_id: string;
    start_time: string;
    total_cost_usd: number;
    prompt_tokens: number;
    completion_tokens: number;
    call_count: number;
    last_model: string;
    last_updated: string;
}

export interface HistoryEntry {
    timestamp: string;
    model: string;
    prompt_tokens: number;
    completion_tokens: number;
    cost_usd: number;
    session_id: string;
}

export async function fetchStatus(): Promise<SessionState> {
    const r = await fetch(`${BASE}/status`);
    if (!r.ok) throw new Error(`status ${r.status}`);
    return r.json();
}

export async function fetchHistory(): Promise<HistoryEntry[]> {
    const r = await fetch(`${BASE}/history`);
    if (!r.ok) throw new Error(`status ${r.status}`);
    return r.json();
}

export async function resetBudget(): Promise<void> {
    const r = await fetch(`${BASE}/reset-budget`, { method: "POST" });
    if (!r.ok) throw new Error(`status ${r.status}`);
}

export function formatUSD(v: number): string {
    return `$${v.toFixed(6)}`;
}

export function formatDate(iso: string): string {
    return new Date(iso).toLocaleString();
}
