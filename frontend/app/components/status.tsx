import { Loader2 } from "lucide-react";
import type { ReactNode } from "react";
import { twMerge } from "tailwind-merge";
import { describeError } from "../lib/describe-error";
import type { ResourceState } from "../lib/use-resource";
import Button from "./button";

export function LoadingState({ label = "載入中" }: { label?: string }) {
    return (
        <div role="status" className="flex items-center justify-center gap-3 py-12 text-copy-muted">
            <Loader2 aria-hidden className="size-6 animate-spin" />
            <span className="font-sans text-base">{label}…</span>
        </div>
    );
}

/** Inline alert line for a failed action; renders nothing without text. */
export function ErrorText({ children }: { children: ReactNode }) {
    if (!children) return null;
    return (
        <p role="alert" className="font-sans text-base text-danger">
            {children}
        </p>
    );
}

/** Polite live-region line for a finished action ("已儲存。"). Stays mounted
 * (hidden while empty) so screen readers announce text set later. */
export function StatusText({
    tone = "neutral",
    children
}: {
    tone?: "neutral" | "success";
    children: ReactNode;
}) {
    return (
        <p
            role="status"
            className={twMerge(
                "font-sans text-base empty:hidden",
                tone === "success" ? "font-medium text-ink" : "text-copy-muted"
            )}
        >
            {children || null}
        </p>
    );
}

/** Submit row for settings forms: the save button (or a custom one passed
 * as children, e.g. wrapped in a ConfirmDialog), the saved notice and the
 * error line. */
export function SaveBar({
    pending,
    saved,
    error,
    label = "儲存",
    pendingLabel = "儲存中…",
    savedText = "已儲存。",
    children
}: {
    pending: boolean;
    saved: boolean;
    error: string | null;
    label?: string;
    pendingLabel?: string;
    savedText?: string;
    children?: ReactNode;
}) {
    return (
        <div className="flex flex-wrap items-center gap-4">
            {children || (
                <Button type="submit" size="base" disabled={pending}>
                    {pending ? pendingLabel : label}
                </Button>
            )}
            <StatusText>{saved ? savedText : null}</StatusText>
            <ErrorText>{error}</ErrorText>
        </div>
    );
}

export function ErrorState({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
    return (
        <div className="flex flex-col items-start gap-4 py-6">
            <ErrorText>{describeError(error)}</ErrorText>
            {onRetry && (
                <Button variant="secondary" size="base" onClick={onRetry}>
                    重新載入
                </Button>
            )}
        </div>
    );
}

export function EmptyState({
    title,
    description,
    action
}: {
    title: string;
    description?: ReactNode;
    action?: ReactNode;
}) {
    return (
        <div className="flex flex-col items-center gap-3 rounded-[var(--radius-panel)] border border-dashed border-ink/20 px-5 py-12 text-center">
            <p className="font-sans text-lg font-medium text-ink">{title}</p>
            {description && (
                <p className="max-w-md font-sans text-base text-copy-muted">{description}</p>
            )}
            {action && <div className="mt-2">{action}</div>}
        </div>
    );
}

/** Loading / error branches for a useResource state; renders `children`
 * with the data once ready. `renderError` handles special errors (e.g.
 * verification_required) and returns undefined to fall back to ErrorState. */
export function Resource<T>({
    state,
    reload,
    loadingLabel,
    renderError,
    children
}: {
    state: ResourceState<T>;
    reload?: () => void;
    loadingLabel?: string;
    renderError?: (error: unknown) => ReactNode | undefined;
    children: (data: T) => ReactNode;
}) {
    if (state.status === "loading") return <LoadingState label={loadingLabel} />;
    if (state.status === "error") {
        return renderError?.(state.error) ?? <ErrorState error={state.error} onRetry={reload} />;
    }
    return children(state.data);
}

export type BadgeTone = "neutral" | "pending" | "success" | "danger";

const badgeTones: Record<BadgeTone, string> = {
    neutral: "bg-ink/8 text-ink",
    pending: "bg-accent-yellow text-ink",
    success: "bg-accent-green text-ink",
    danger: "bg-danger/10 text-danger"
};

export function StatusBadge({
    tone = "neutral",
    className,
    children
}: {
    tone?: BadgeTone;
    className?: string;
    children: ReactNode;
}) {
    return (
        <span
            className={twMerge(
                "inline-flex shrink-0 items-center rounded-full px-3 py-1 font-sans text-sm font-medium whitespace-nowrap",
                badgeTones[tone],
                className
            )}
        >
            {children}
        </span>
    );
}
