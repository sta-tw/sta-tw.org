"use client";

import { ChevronDown, Download, FileText, Folder } from "lucide-react";
import Link from "next/link";
import { Collapsible } from "radix-ui";
import { useState, type ReactNode } from "react";
import { twMerge } from "tailwind-merge";
import {
    getPortfolioItemDownload,
    portfolioLabel,
    portfolioUpdatedAt,
    portfolioVisibilityLabels,
    type Portfolio,
    type PortfolioFileStatus
} from "../lib/api/portfolio";
import { formatBytes, formatNumericDate } from "../lib/format";
import { userHref } from "../lib/routes";
import { useAction } from "../lib/use-resource";
import { ErrorText } from "./status";

const iconButtonClass =
    "inline-flex size-10 shrink-0 cursor-pointer items-center justify-center rounded-[var(--radius-control)] text-ink focus-ring transition-colors hover:bg-ink/5 disabled:pointer-events-none disabled:opacity-50";

const fileStatusLabels: Record<PortfolioFileStatus, string> = {
    hidden: "未公開",
    pending_review: "審核中",
    published: "已公開",
    unpublished: "已下架",
    rejected: "未通過"
};

/** A project with its file versions; viewer actions wait for backend support. */
export default function PortfolioCard({
    portfolio,
    showAuthor = true,
    viewerActions = false,
    status,
    actions,
    defaultOpen = false
}: {
    portfolio: Portfolio;
    showAuthor?: boolean;
    viewerActions?: boolean;
    status?: ReactNode;
    actions?: ReactNode;
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
                        <span className="block min-w-0 flex-1">
                            <span className="block font-sans text-lg leading-snug font-medium break-words text-ink sm:text-xl">
                                {label || portfolio.title}
                                {showAuthor && portfolio.author_username && (
                                    <span className="text-copy-muted">
                                        {" "}
                                        — {portfolio.author_name}
                                    </span>
                                )}
                            </span>
                            <span className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 font-sans text-sm text-copy-muted">
                                <Folder aria-hidden className="size-4 shrink-0" />
                                <span>{portfolio.items.length} 個檔案版本</span>
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
                        <span className="sr-only">{open ? "收合版本" : "展開版本"}</span>
                    </Collapsible.Trigger>
                    {viewerActions && (
                        <p className="font-sans text-xs text-copy-muted">按讚與收藏尚未開放</p>
                    )}
                </div>
                <Collapsible.Content className="border-t border-ink/10 px-4 pb-4 sm:px-5 sm:pb-5">
                    <ol className="divide-y divide-ink/10">
                        {portfolio.items.map((item) => (
                            <li key={item.id} className="flex items-center gap-3 py-3">
                                <FileText aria-hidden className="size-5 shrink-0 text-copy-muted" />
                                <div className="min-w-0 flex-1">
                                    <p className="font-sans text-base break-words text-ink">
                                        {item.version_number > 0
                                            ? `第 ${item.version_number} 版 · `
                                            : ""}
                                        {item.name}
                                    </p>
                                    <p className="font-sans text-sm text-copy-muted">
                                        {item.mime_type !== "application/octet-stream" &&
                                            `${item.mime_type} · `}
                                        {item.file_size_bytes > 0
                                            ? `${formatBytes(item.file_size_bytes)} · `
                                            : ""}
                                        {fileStatusLabels[item.status]} · 更新於{" "}
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
                    {showAuthor && portfolio.author_username && (
                        <p className="mt-2 font-sans text-sm text-copy-muted">
                            分享者：
                            <Link
                                href={userHref(portfolio.author_username)}
                                className="text-ink underline-offset-4 hover:underline"
                            >
                                {portfolio.author_name || portfolio.author_username}
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

/** Opens a short-lived download URL in a new tab. */
function useDownload() {
    const { pending, error, run } = useAction();

    function start(fileID: string) {
        const tab = window.open("", "_blank");
        void run(fileID, async () => {
            try {
                const { url } = await getPortfolioItemDownload(fileID);
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
