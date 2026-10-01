"use client";

// Shared building blocks for every /admin/* page — pulled out of
// admin-shell.tsx/page.tsx/admissions-view.tsx/audit-log-view.tsx/
// mail-routes-view.tsx/users-view.tsx, which had each independently
// redefined inputClass, describeError, Th, formatDate, and a Dialog frame.
// Reuses the site's own design tokens (radius-panel/radius-small/
// shadow-card, ink/surface/copy-muted) — no new colors or fonts.

import { Dialog } from "radix-ui";
import { twMerge } from "tailwind-merge";
import { AlertCircle, Loader2, X } from "lucide-react";
import Button from "../components/button";
import { ApiError } from "../lib/api/types";

export function describeError(cause: unknown): string {
    if (cause instanceof ApiError) return cause.message || `發生錯誤（${cause.code}）`;
    return "發生未知錯誤，請稍後再試。";
}

export function formatDate(iso?: string): string {
    if (!iso) return "—";
    try {
        return new Date(iso).toLocaleString("zh-TW", { dateStyle: "medium", timeStyle: "short" });
    } catch {
        return iso;
    }
}

export const inputClass =
    "rounded-[var(--radius-small)] border border-ink/15 bg-surface px-3 py-2 font-sans text-sm text-ink outline-none focus:border-ink/40";

export const textareaClass = twMerge(inputClass, "min-h-20 w-full resize-y");

/** The card container class repeated (with slightly different borders/
 * shadows each time) across every admin page. Exported as a plain string too
 * so a non-div element (e.g. a `<form>`) can still use it directly. */
export const panelClassName = "rounded-[var(--radius-panel)] border border-ink/10 bg-surface p-5 shadow-[var(--shadow-card)]";

export function AdminPanel({
    className,
    children
}: {
    className?: string;
    children: React.ReactNode;
}) {
    return <div className={twMerge(panelClassName, className)}>{children}</div>;
}

export function Th({ children }: { children?: React.ReactNode }) {
    return (
        <th className="px-4 py-3 text-left font-sans text-xs font-bold tracking-wide text-copy-muted uppercase">
            {children}
        </th>
    );
}

export function Td({ children, className }: { children: React.ReactNode; className?: string }) {
    return <td className={twMerge("px-4 py-3 font-sans text-sm text-ink", className)}>{children}</td>;
}

/** Wraps a `<table>` in the scroll/rounded/shadow shell every admin table
 * uses, plus the empty-state paragraph that goes under it. */
export function AdminTable({
    minWidth = 720,
    isEmpty,
    emptyLabel = "沒有符合條件的資料。",
    children
}: {
    minWidth?: number;
    isEmpty?: boolean;
    emptyLabel?: string;
    children: React.ReactNode;
}) {
    return (
        <div className="overflow-x-auto rounded-[var(--radius-panel)] border border-ink/10 bg-surface shadow-[var(--shadow-card)]">
            <table className="w-full font-sans text-sm" style={{ minWidth }}>
                <tbody className="[&>tr]:border-b [&>tr]:border-ink/5 [&>tr:last-child]:border-0">
                    {children}
                </tbody>
            </table>
            {isEmpty ? <p className="p-6 text-center font-sans text-sm text-copy-muted">{emptyLabel}</p> : null}
        </div>
    );
}

export type BadgeTone = "neutral" | "positive" | "warning" | "negative";

export const badgePillClass = "inline-flex rounded-full px-3 py-1 font-sans text-xs font-bold";

export const badgeToneClasses: Record<BadgeTone, string> = {
    neutral: "bg-ink/10 text-copy-muted",
    positive: "bg-accent-green-strong text-ink",
    warning: "bg-amber-100 text-amber-700",
    negative: "bg-red-100 text-red-700"
};

/** Status pill — the one visual vocabulary every admin page's ad hoc
 * "is this good/bad/pending" badge should map onto. Pages whose status set
 * doesn't fit the {tone, children} shape (e.g. a badge-shaped dialog
 * trigger button) can still reuse the exact same colors via
 * badgePillClass/badgeToneClasses directly — see admissions-view.tsx. */
export function Badge({ tone = "neutral", children }: { tone?: BadgeTone; children: React.ReactNode }) {
    return <span className={twMerge(badgePillClass, badgeToneClasses[tone])}>{children}</span>;
}

export function LoadingState({ label = "載入中…" }: { label?: string }) {
    return (
        <div className="flex items-center justify-center gap-2 p-6 font-sans text-sm text-copy-muted">
            <Loader2 aria-hidden className="h-4 w-4 animate-spin" />
            {label}
        </div>
    );
}

export function EmptyState({ label }: { label: string }) {
    return <p className="p-6 text-center font-sans text-sm text-copy-muted">{label}</p>;
}

export function ErrorText({ children }: { children: React.ReactNode }) {
    return (
        <p className="flex items-start gap-1.5 font-sans text-sm text-red-600">
            <AlertCircle aria-hidden className="mt-0.5 h-4 w-4 shrink-0" />
            {children}
        </p>
    );
}

/** The Radix Dialog "frame" (overlay + centered card + title row + close
 * button) that users-view.tsx and admissions-view.tsx each copy-pasted
 * verbatim per dialog. Callers supply the body via children. */
export function AdminDialog({
    open,
    onOpenChange,
    title,
    description,
    maxWidth = "max-w-md",
    children
}: {
    open: boolean;
    onOpenChange: (open: boolean) => void;
    title: React.ReactNode;
    description?: React.ReactNode;
    maxWidth?: string;
    children: React.ReactNode;
}) {
    return (
        <Dialog.Root open={open} onOpenChange={onOpenChange}>
            <Dialog.Portal>
                <Dialog.Overlay className="fixed inset-0 z-[80] bg-ink/40" />
                <Dialog.Content
                    className={twMerge(
                        "fixed top-1/2 left-1/2 z-[90] w-[calc(100vw-2.5rem)] -translate-x-1/2 -translate-y-1/2 rounded-[var(--radius-panel)] bg-surface p-6 shadow-[var(--shadow-card)]",
                        maxWidth
                    )}
                >
                    <div className="flex items-start justify-between gap-4">
                        <div>
                            <Dialog.Title className="font-serif text-xl text-ink">{title}</Dialog.Title>
                            {description ? (
                                <p className="mt-1 font-sans text-sm text-copy-muted">{description}</p>
                            ) : null}
                        </div>
                        <Dialog.Close asChild>
                            <button
                                type="button"
                                aria-label="關閉"
                                className="rounded-[var(--radius-small)] p-1.5 text-copy-muted hover:bg-ink/5 hover:text-ink"
                            >
                                <X aria-hidden className="h-5 w-5" />
                            </button>
                        </Dialog.Close>
                    </div>
                    <div className="mt-5 flex flex-col gap-3">{children}</div>
                </Dialog.Content>
            </Dialog.Portal>
        </Dialog.Root>
    );
}

/** Replaces the window.confirm() call in mail-routes-view.tsx's delete flow
 * with the same Dialog language every other destructive action already
 * uses elsewhere in the admin panel. */
export function ConfirmDialog({
    open,
    onOpenChange,
    title,
    description,
    confirmLabel = "確認",
    pending = false,
    onConfirm
}: {
    open: boolean;
    onOpenChange: (open: boolean) => void;
    title: React.ReactNode;
    description?: React.ReactNode;
    confirmLabel?: string;
    pending?: boolean;
    onConfirm: () => void;
}) {
    return (
        <AdminDialog open={open} onOpenChange={onOpenChange} title={title} description={description}>
            <div className="flex justify-end gap-2 pt-2">
                <Button type="button" variant="secondary" className="h-10 px-4 text-sm" onClick={() => onOpenChange(false)}>
                    取消
                </Button>
                <Button
                    type="button"
                    variant="danger"
                    className="h-10 px-4 font-sans text-sm"
                    disabled={pending}
                    onClick={onConfirm}
                >
                    {pending ? "處理中…" : confirmLabel}
                </Button>
            </div>
        </AdminDialog>
    );
}
