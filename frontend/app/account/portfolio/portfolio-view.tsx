"use client";

import Link from "next/link";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import Button from "../../components/button";
import ConfirmDialog from "../../components/confirm-dialog";
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
    deletePortfolio,
    listMyPortfolios,
    portfolioLabel,
    submitPortfolio,
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
    { visibility: "private", title: "私人備審", empty: "只有你看得到，適合整理草稿或備份。" },
    {
        visibility: "exchange",
        title: "交換備審",
        empty: "與已驗證的特選生互相交換，分享後也能看到別人的交換備審。"
    },
    { visibility: "public", title: "公開備審", empty: "審核通過後，所有人都能瀏覽與下載。" }
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

    const remove = (portfolio: Portfolio) =>
        act(
            portfolio.id,
            async () => {
                await deletePortfolio(portfolio.id);
                setData((items) => items.filter((item) => item.id !== portfolio.id));
            },
            "已刪除備審。"
        );

    return (
        <AccountPage title="備審上傳" description="整理你的備審資料，也可以分享給學弟妹參考。">
            <div className="flex flex-col divide-y divide-ink/10 rounded-[var(--radius-panel)] border border-ink/10 bg-surface">
                <div className="flex flex-wrap items-center justify-between gap-3 p-4 sm:px-6">
                    <p className="font-sans text-base text-ink">新增一份備審</p>
                    <Button asChild size="sm" className="gap-2">
                        <Link href={portfolioEditorHref()}>
                            <Plus aria-hidden className="size-4" />
                            新增備審
                        </Link>
                    </Button>
                </div>
                <div className="flex flex-wrap items-center justify-between gap-3 p-4 sm:px-6">
                    <p className="font-sans text-base text-ink">沒有想法？來看看別人分享的備審</p>
                    <Button asChild variant="secondary" size="sm">
                        <Link href="/portfolio">看看備審</Link>
                    </Button>
                </div>
            </div>

            <ErrorText>{error}</ErrorText>
            <StatusText>{notice}</StatusText>

            <Resource state={state} reload={reload}>
                {(portfolios) =>
                    portfolios.length === 0 ? (
                        <EmptyState
                            title="還沒有上傳備審"
                            description="一份備審對應一個申請校系，可以包含自傳、讀書計畫等多個 PDF。"
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
                                                        onDelete={() => remove(portfolio)}
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
    onDelete
}: {
    portfolio: Portfolio;
    busy: boolean;
    onSubmit: () => void;
    onDelete: () => void;
}) {
    const shared = portfolio.visibility !== "private";
    const review = reviewBadge[portfolio.review_status];
    const canSubmit =
        shared && (portfolio.review_status === "draft" || portfolio.review_status === "rejected");

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
                            編輯
                        </Link>
                    </Button>
                    {canSubmit && (
                        <SubmitReviewDialog
                            trigger={
                                <Button size="sm" disabled={busy}>
                                    送出審核
                                </Button>
                            }
                            onConfirm={onSubmit}
                        />
                    )}
                    <ConfirmDialog
                        trigger={
                            <Button variant="secondary" size="sm" className="gap-2" disabled={busy}>
                                <Trash2 aria-hidden className="size-4" />
                                刪除
                            </Button>
                        }
                        title="刪除這份備審？"
                        description={`${portfolioLabel(portfolio)} 的 ${portfolio.items.length} 個檔案都會一併刪除，無法復原。`}
                        confirmLabel="刪除"
                        onConfirm={onDelete}
                    />
                </>
            }
        />
    );
}
