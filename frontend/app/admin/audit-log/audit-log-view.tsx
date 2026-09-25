"use client";

import { Fragment, useEffect, useState } from "react";
import { ChevronDown, ChevronUp, Search } from "lucide-react";
import Button from "../../components/button";
import { listAuditLog, type AuditRow } from "../../lib/api/admin";
import { ApiError } from "../../lib/api/types";
import { useAdmin } from "../admin-context";

const inputClass =
    "rounded-[var(--radius-small)] border border-ink/15 bg-surface px-3 py-2 font-sans text-sm text-ink outline-none focus:border-ink/40";

function describeError(cause: unknown): string {
    if (cause instanceof ApiError) return cause.message || `發生錯誤（${cause.code}）`;
    return "發生未知錯誤，請稍後再試。";
}

export default function AuditLogView() {
    const { mfaCode } = useAdmin();
    const [entityType, setEntityType] = useState("");
    const [entityKey, setEntityKey] = useState("");
    const [action, setAction] = useState("");
    const [actor, setActor] = useState("");

    const [rows, setRows] = useState<AuditRow[] | null>(null);
    const [nextCursor, setNextCursor] = useState("");
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [expanded, setExpanded] = useState<number | null>(null);

    async function load(cursor?: string) {
        setLoading(true);
        setError(null);
        try {
            const page = await listAuditLog(
                {
                    entity_type: entityType.trim() || undefined,
                    entity_key: entityKey.trim() || undefined,
                    action: action.trim() || undefined,
                    actor: actor.trim() || undefined,
                    cursor
                },
                mfaCode || undefined
            );
            setRows((prev) => (cursor && prev ? [...prev, ...page.data] : page.data));
            setNextCursor(page.next_cursor);
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setLoading(false);
        }
    }

    useEffect(() => {
        // load() only setStates after its awaited fetch resolves, never
        // synchronously — safe to call directly on mount / when the MFA
        // grant becomes available.
        // eslint-disable-next-line react-hooks/set-state-in-effect
        void load();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [mfaCode]);

    function handleFilterSubmit(event: React.FormEvent) {
        event.preventDefault();
        void load();
    }

    return (
        <div className="flex flex-col gap-6">
            <h1 className="font-serif text-hero-subtitle text-ink">稽核紀錄</h1>

            <form onSubmit={handleFilterSubmit} className="flex flex-wrap items-end gap-3">
                <Field label="實體類型">
                    <input
                        className={inputClass}
                        placeholder="例如 account"
                        value={entityType}
                        onChange={(e) => setEntityType(e.target.value)}
                    />
                </Field>
                <Field label="實體 Key">
                    <input
                        className={inputClass}
                        value={entityKey}
                        onChange={(e) => setEntityKey(e.target.value)}
                    />
                </Field>
                <Field label="動作">
                    <input
                        className={inputClass}
                        placeholder="例如 account.suspended"
                        value={action}
                        onChange={(e) => setAction(e.target.value)}
                    />
                </Field>
                <Field label="操作者 UUID">
                    <input
                        className={inputClass}
                        value={actor}
                        onChange={(e) => setActor(e.target.value)}
                    />
                </Field>
                <Button type="submit" disabled={loading} className="h-10 gap-2 px-5 text-sm">
                    <Search aria-hidden className="h-4 w-4" />
                    篩選
                </Button>
            </form>

            {error ? <p className="font-sans text-sm text-red-600">{error}</p> : null}

            <div className="overflow-x-auto rounded-[var(--radius-panel)] bg-surface shadow-[var(--shadow-card)]">
                <table className="w-full min-w-[760px] font-sans text-sm">
                    <thead>
                        <tr className="border-b border-ink/10 text-left text-copy-muted">
                            <Th>時間</Th>
                            <Th>動作</Th>
                            <Th>實體</Th>
                            <Th>操作者</Th>
                            <Th>原因</Th>
                            <Th />
                        </tr>
                    </thead>
                    <tbody>
                        {rows?.map((row) => (
                            <Fragment key={row.id}>
                                <tr className="border-b border-ink/5 last:border-0">
                                    <td className="px-4 py-3 whitespace-nowrap text-copy-muted">
                                        {formatDate(row.created_at)}
                                    </td>
                                    <td className="px-4 py-3 font-mono text-xs text-ink">
                                        {row.action}
                                    </td>
                                    <td className="px-4 py-3 text-copy-muted">
                                        {row.entity_type} · {row.entity_key}
                                    </td>
                                    <td className="px-4 py-3 font-mono text-xs text-copy-muted">
                                        {row.actor_account_id
                                            ? row.actor_account_id.slice(0, 8)
                                            : "系統"}
                                    </td>
                                    <td className="max-w-64 truncate px-4 py-3 text-copy-muted">
                                        {row.reason}
                                    </td>
                                    <td className="px-4 py-3">
                                        {row.before_data || row.after_data ? (
                                            <button
                                                type="button"
                                                onClick={() =>
                                                    setExpanded(expanded === row.id ? null : row.id)
                                                }
                                                className="flex items-center gap-1 font-sans text-xs text-ink underline underline-offset-2"
                                            >
                                                詳細
                                                {expanded === row.id ? (
                                                    <ChevronUp
                                                        aria-hidden
                                                        className="h-3.5 w-3.5"
                                                    />
                                                ) : (
                                                    <ChevronDown
                                                        aria-hidden
                                                        className="h-3.5 w-3.5"
                                                    />
                                                )}
                                            </button>
                                        ) : null}
                                    </td>
                                </tr>
                                {expanded === row.id ? (
                                    <tr className="border-b border-ink/5 bg-ink/[0.02]">
                                        <td colSpan={6} className="px-4 py-3">
                                            <div className="grid gap-3 sm:grid-cols-2">
                                                {row.before_data ? (
                                                    <JsonBlock
                                                        title="變更前"
                                                        value={row.before_data}
                                                    />
                                                ) : null}
                                                {row.after_data ? (
                                                    <JsonBlock
                                                        title="變更後"
                                                        value={row.after_data}
                                                    />
                                                ) : null}
                                            </div>
                                        </td>
                                    </tr>
                                ) : null}
                            </Fragment>
                        ))}
                    </tbody>
                </table>
                {rows !== null && rows.length === 0 ? (
                    <p className="p-6 text-center font-sans text-copy-muted">
                        沒有符合條件的紀錄。
                    </p>
                ) : null}
            </div>

            {nextCursor ? (
                <Button
                    onClick={() => void load(nextCursor)}
                    disabled={loading}
                    className="w-fit self-center border border-ink/15 bg-surface px-6 text-ink hover:bg-ink/5"
                >
                    {loading ? "載入中…" : "載入更多"}
                </Button>
            ) : null}
        </div>
    );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
    return (
        <label className="flex flex-col gap-1">
            <span className="font-sans text-xs text-copy-muted">{label}</span>
            {children}
        </label>
    );
}

function Th({ children }: { children?: React.ReactNode }) {
    return (
        <th className="px-4 py-3 font-sans text-xs font-bold tracking-wide uppercase">
            {children}
        </th>
    );
}

function JsonBlock({ title, value }: { title: string; value: unknown }) {
    return (
        <div className="rounded-[var(--radius-small)] border border-ink/10 bg-surface p-3">
            <p className="font-sans text-xs font-bold text-copy-muted">{title}</p>
            <pre className="mt-1 overflow-x-auto font-mono text-xs text-ink">
                {JSON.stringify(value, null, 2)}
            </pre>
        </div>
    );
}

function formatDate(iso: string): string {
    try {
        return new Date(iso).toLocaleString("zh-TW", { dateStyle: "medium", timeStyle: "short" });
    } catch {
        return iso;
    }
}
