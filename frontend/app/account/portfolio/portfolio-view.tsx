"use client";

import Link from "next/link";
import { Pencil, Plus } from "lucide-react";
import { useState } from "react";
import Button from "../../components/button";
import PortfolioCard from "../../components/portfolio-card";
import {
    EmptyState,
    ErrorText,
    Resource,
    StatusBadge,
    StatusText,
    type BadgeTone
} from "../../components/status";
import {
    listMyPortfolios,
    submitPortfolio,
    unpublishPortfolioFile,
    type Portfolio,
    type PortfolioReviewStatus,
    type PortfolioVisibility
} from "../../lib/api/portfolio";
import { portfolioEditorHref } from "../../lib/routes";
import { useAction, useResource } from "../../lib/use-resource";
import AccountPage from "../account-page";
import SubmitReviewDialog from "./submit-review-dialog";

const loadMine = (signal: AbortSignal) => listMyPortfolios({ signal }).then(({ data }) => data);

const sections: { visibility: PortfolioVisibility; title: string; empty: string }[] = [
    {
        visibility: "private",
        title: "尚未公開",
        empty: "上傳的檔案會先保持未公開，送審通過後才會出現在個人頁面。"
    },
    {
        visibility: "public",
        title: "公開備審",
        empty: "審核通過的檔案會顯示在你的公開個人頁面。"
    }
];

const reviewBadge: Record<PortfolioReviewStatus, { label: string; tone: BadgeTone }> = {
    draft: { label: "尚未送審", tone: "neutral" },
    pending_review: { label: "審核中", tone: "pending" },
    approved: { label: "已分享", tone: "success" },
    rejected: { label: "未通過審核", tone: "danger" }
};

export default function PortfolioView() {
    const { state, reload, setData } = useResource(loadMine);
    const { pending, error, run } = useAction();
    const [notice, setNotice] = useState<string | null>(null);

    async function act(id: string, action: () => Promise<void>, success: string) {
        setNotice(null);
        if (await run(id, action)) setNotice(success);
    }

    const submit = (portfolio: Portfolio) =>
        act(
            portfolio.id,
            async () => {
                const { data } = await submitPortfolio(portfolio.id);
                setData((items) => items.map((item) => (item.id === data.id ? data : item)));
            },
            "已送出審核，通過後就會分享出去。"
        );

    const unpublish = (portfolio: Portfolio) =>
        act(
            portfolio.id,
            async () => {
                const published = portfolio.items.filter((item) => item.status === "published");
                for (const item of published) await unpublishPortfolioFile(item.id);
                await reload();
            },
            "已將公開版本下架。"
        );

    return (
        <AccountPage title="備審上傳" description="管理已確認申請校系的備審檔案版本與送審狀態。">
            <div className="flex flex-col divide-y divide-ink/10 rounded-[var(--radius-panel)] border border-ink/10 bg-surface">
                <div className="flex flex-wrap items-center justify-between gap-3 p-4 sm:px-6">
                    <p className="font-sans text-base text-ink">新增備審專案</p>
                    <Button asChild size="sm" className="gap-2">
                        <Link href={portfolioEditorHref()}>
                            <Plus aria-hidden className="size-4" />
                            新增專案
                        </Link>
                    </Button>
                </div>
                <div className="flex flex-wrap items-center justify-between gap-3 p-4 sm:px-6">
                    <p className="font-sans text-base text-ink">
                        全站備審瀏覽功能尚未由正式後端提供。
                    </p>
                </div>
            </div>

            <ErrorText>{error}</ErrorText>
            <StatusText>{notice}</StatusText>

            <Resource state={state} reload={reload}>
                {(portfolios) =>
                    portfolios.length === 0 ? (
                        <EmptyState
                            title="還沒有備審專案"
                            description="每筆正式申請可建立一個備審專案；上傳檔案會依序保存為版本。"
                        />
                    ) : (
                        sections.map((section) => {
                            const items = portfolios.filter(
                                (item) => item.visibility === section.visibility
                            );
                            return (
                                <section
                                    key={section.visibility}
                                    aria-labelledby={`portfolio-${section.visibility}`}
                                    className="flex flex-col gap-3"
                                >
                                    <h2
                                        id={`portfolio-${section.visibility}`}
                                        className="font-sans text-2xl font-medium text-ink"
                                    >
                                        {section.title}
                                    </h2>
                                    {items.length === 0 ? (
                                        <p className="font-sans text-base text-copy-muted">
                                            {section.empty}
                                        </p>
                                    ) : (
                                        <ul className="flex flex-col gap-3">
                                            {items.map((portfolio) => (
                                                <li key={portfolio.id}>
                                                    <OwnPortfolio
                                                        portfolio={portfolio}
                                                        busy={pending === portfolio.id}
                                                        onSubmit={() => submit(portfolio)}
                                                        onUnpublish={() => unpublish(portfolio)}
                                                    />
                                                </li>
                                            ))}
                                        </ul>
                                    )}
                                </section>
                            );
                        })
                    )
                }
            </Resource>
        </AccountPage>
    );
}

function OwnPortfolio({
    portfolio,
    busy,
    onSubmit,
    onUnpublish
}: {
    portfolio: Portfolio;
    busy: boolean;
    onSubmit: () => void;
    onUnpublish: () => void;
}) {
    const shared = portfolio.visibility === "public";
    const review = reviewBadge[portfolio.review_status];
    const newest = portfolio.items[0];
    const canSubmit = Boolean(
        newest && ["hidden", "unpublished", "rejected"].includes(newest.status)
    );
    const canUnpublish = portfolio.items.some((item) => item.status === "published");

    return (
        <PortfolioCard
            portfolio={portfolio}
            showAuthor={false}
            viewerActions={false}
            status={
                shared && (
                    <>
                        <StatusBadge tone={review.tone}>{review.label}</StatusBadge>
                        {portfolio.review_status === "rejected" && portfolio.rejection_reason && (
                            <span className="font-sans text-sm text-danger">
                                原因：{portfolio.rejection_reason}
                            </span>
                        )}
                    </>
                )
            }
            actions={
                <>
                    <Button asChild variant="secondary" size="sm" className="gap-2">
                        <Link href={portfolioEditorHref(portfolio.id)}>
                            <Pencil aria-hidden className="size-4" />
                            上傳新版本
                        </Link>
                    </Button>
                    {canSubmit && (
                        <SubmitReviewDialog
                            trigger={
                                <Button size="sm" disabled={busy}>
                                    送出最新版本審核
                                </Button>
                            }
                            onConfirm={onSubmit}
                        />
                    )}
                    {canUnpublish && (
                        <Button variant="secondary" size="sm" disabled={busy} onClick={onUnpublish}>
                            下架公開版本
                        </Button>
                    )}
                </>
            }
        />
    );
}
