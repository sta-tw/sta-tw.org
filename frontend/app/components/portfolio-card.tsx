"use client";

import { Bookmark, ChevronDown, Download, FileText, Folder, Heart } from "lucide-react";
import Link from "next/link";
import { Collapsible } from "radix-ui";
import { usePathname, useRouter } from "next/navigation";
import { useState, type ReactNode } from "react";
import { twMerge } from "tailwind-merge";
import {
    getPortfolioItemDownload,
    likePortfolio,
    portfolioLabel,
    portfolioUpdatedAt,
    portfolioVisibilityLabels,
    removePortfolioBookmark,
    savePortfolioBookmark,
    unlikePortfolio,
    type Portfolio
} from "../lib/api/portfolio";
import { useCurrentAccount } from "../lib/current-account";
import { formatBytes, formatNumericDate } from "../lib/format";
import { loginHref, userHref } from "../lib/routes";
import { useAction } from "../lib/use-resource";
import { ErrorText } from "./status";

const iconButtonClass =
    "inline-flex size-10 shrink-0 cursor-pointer items-center justify-center rounded-[var(--radius-control)] text-ink focus-ring transition-colors hover:bg-ink/5 disabled:pointer-events-none disabled:opacity-50";

/** A 備審 as a collapsible row: 校系, item count and visibility up top,
 * the PDFs (with downloads) when expanded. Viewers get like / bookmark
 * toggles; the owner's pages pass `status` and `actions` instead. */
export default function PortfolioCard({
    portfolio,
    showAuthor = true,
    viewerActions = true,
    status,
    actions,
    onSavedChange,
    defaultOpen = false
}: {
    portfolio: Portfolio;
    showAuthor?: boolean;
    /** Like / bookmark toggles; off on the owner's own pages. */
    viewerActions?: boolean;
    /** Badges beside the meta line, e.g. the review status. */
    status?: ReactNode;
    /** Owner actions shown under the items (edit, submit, delete). */
    actions?: ReactNode;
    onSavedChange?: (saved: boolean) => void;
    defaultOpen?: boolean;
}) {
    const [open, setOpen] = useState(defaultOpen);
    const download = useDownload();
    const label = portfolioLabel(portfolio);

    return (
        <Collapsible.Root open={open} onOpenChange={setOpen}>
            <article
                aria-label={label}
                className={twMerge(
                    "rounded-[var(--radius-panel)] border border-ink/10 bg-surface transition-colors",
                    open && "border-ink/20"
                )}
            >
                <div className="flex items-start gap-2 p-4 sm:items-center sm:p-5">
                    <Collapsible.Trigger className="group flex min-w-0 flex-1 cursor-pointer items-start gap-3 rounded-[var(--radius-control)] text-left focus-ring">
                        {/* Spans only: a button may hold phrasing content alone. */}
                        <span className="block min-w-0 flex-1">
                            <span className="block font-sans text-lg leading-snug font-medium break-words text-ink sm:text-xl">
                                {label}
                                {showAuthor && (
                                    <span className="text-copy-muted">
                                        {" "}
                                        - {portfolio.author_name}
                                    </span>
                                )}
                            </span>
                            <span className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 font-sans text-sm text-copy-muted">
                                <Folder aria-hidden className="size-4 shrink-0" />
                                <span>{portfolio.items.length} 個項目</span>
                                <span aria-hidden>·</span>
                                <span>
                                    {portfolioVisibilityLabels[portfolio.visibility]}（
                                    {portfolio.academic_year}）
                                </span>
                                <span aria-hidden>·</span>
                                <span>
                                    更新於{" "}
                                    <time dateTime={portfolioUpdatedAt(portfolio)}>
                                        {formatNumericDate(portfolioUpdatedAt(portfolio))}
                                    </time>
                                </span>
                            </span>
                            {status && <span className="mt-2 flex flex-wrap gap-2">{status}</span>}
                        </span>
                        <ChevronDown
                            aria-hidden
                            className="mt-1 size-5 shrink-0 text-copy-muted transition-transform group-data-[state=open]:rotate-180 sm:mt-0 sm:self-center"
                        />
                        <span className="sr-only">{open ? "收合項目" : "展開項目"}</span>
                    </Collapsible.Trigger>
                    {viewerActions && (
                        <ViewerActions portfolio={portfolio} onSavedChange={onSavedChange} />
                    )}
                </div>
                <Collapsible.Content className="border-t border-ink/10 px-4 pb-4 sm:px-5 sm:pb-5">
                    <ol className="divide-y divide-ink/10">
                        {portfolio.items.map((item, index) => (
                            <li key={item.id} className="flex items-center gap-3 py-3">
                                <FileText aria-hidden className="size-5 shrink-0 text-copy-muted" />
                                <div className="min-w-0 flex-1">
                                    <p className="font-sans text-base break-words text-ink">
                                        {String(index + 1).padStart(2, "0")} - {item.name}
                                    </p>
                                    <p className="font-sans text-sm text-copy-muted">
                                        PDF · {formatBytes(item.file_size_bytes)} · 更新於{" "}
                                        <time dateTime={item.updated_at}>
                                            {formatNumericDate(item.updated_at)}
                                        </time>
                                    </p>
                                </div>
                                <button
                                    type="button"
                                    aria-label={`下載 ${item.name}`}
                                    className={iconButtonClass}
                                    disabled={download.pending === item.id}
                                    onClick={() => download.start(item.id)}
                                >
                                    <Download aria-hidden className="size-5" />
                                </button>
                            </li>
                        ))}
                    </ol>
                    <ErrorText>{download.error}</ErrorText>
                    {showAuthor && (
                        <p className="mt-2 font-sans text-sm text-copy-muted">
                            分享者：
                            <Link
                                href={userHref(portfolio.author_username)}
                                className="text-ink underline-offset-4 hover:underline"
                            >
                                {portfolio.author_name}
                            </Link>
                        </p>
                    )}
                    {actions && (
                        <div className="mt-3 flex flex-wrap gap-2 border-t border-ink/10 pt-4">
                            {actions}
                        </div>
                    )}
                </Collapsible.Content>
            </article>
        </Collapsible.Root>
    );
}

function ViewerActions({
    portfolio,
    onSavedChange
}: {
    portfolio: Portfolio;
    onSavedChange?: (saved: boolean) => void;
}) {
    const { status } = useCurrentAccount();
    const router = useRouter();
    const pathname = usePathname();
    const [liked, setLiked] = useState(portfolio.liked);
    const [likeCount, setLikeCount] = useState(portfolio.like_count);
    const [saved, setSaved] = useState(portfolio.saved);
    const { pending, error, run } = useAction();

    const signedIn = status === "authenticated";
    const requireSignIn = () => {
        router.push(loginHref(pathname));
    };

    const toggleLike = () => {
        if (!signedIn) return requireSignIn();
        const next = !liked;
        void run("like", async () => {
            await (next ? likePortfolio : unlikePortfolio)(portfolio.id);
            setLiked(next);
            setLikeCount((count) => Math.max(0, count + (next ? 1 : -1)));
        });
    };

    const toggleSaved = () => {
        if (!signedIn) return requireSignIn();
        const next = !saved;
        void run("save", async () => {
            await (next ? savePortfolioBookmark : removePortfolioBookmark)(portfolio.id);
            setSaved(next);
            onSavedChange?.(next);
        });
    };

    return (
        <div className="flex shrink-0 flex-col items-end gap-1">
            <div className="flex items-center gap-1">
                <button
                    type="button"
                    aria-pressed={liked}
                    aria-label={`按讚（${likeCount}）`}
                    title="按讚"
                    className={twMerge(iconButtonClass, "w-auto min-w-10 gap-1 px-2")}
                    disabled={pending !== null}
                    onClick={toggleLike}
                >
                    <Heart
                        aria-hidden
                        className={twMerge("size-5", liked && "fill-danger text-danger")}
                    />
                    {likeCount > 0 && (
                        <span aria-hidden className="font-sans text-sm text-copy-muted">
                            {likeCount}
                        </span>
                    )}
                </button>
                <button
                    type="button"
                    aria-pressed={saved}
                    aria-label={saved ? "已儲存，點擊取消儲存" : "儲存"}
                    title={saved ? "取消儲存" : "儲存"}
                    className={iconButtonClass}
                    disabled={pending !== null}
                    onClick={toggleSaved}
                >
                    <Bookmark aria-hidden className={twMerge("size-5", saved && "fill-ink")} />
                </button>
            </div>
            <ErrorText>{error}</ErrorText>
        </div>
    );
}

/** Opens an item's presigned download URL in a new tab. */
function useDownload() {
    const { pending, error, run } = useAction();

    function start(itemID: string) {
        // Open the tab synchronously so popup blockers allow it, then point
        // it at the URL once the request resolves.
        const tab = window.open("", "_blank");
        void run(itemID, async () => {
            try {
                const { url } = await getPortfolioItemDownload(itemID);
                if (tab) {
                    tab.opener = null;
                    tab.location.href = url;
                } else {
                    window.location.assign(url);
                }
            } catch (cause) {
                tab?.close();
                throw cause;
            }
        });
    }

    return { pending, error, start };
}
