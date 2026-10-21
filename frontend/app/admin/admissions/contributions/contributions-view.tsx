"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Check, Download, ExternalLink, Eye, Loader2, X } from "lucide-react";
import Button from "../../../components/button";
import {
    getAdminAdmissionsContributionContent,
    listAdminAdmissionsContributions,
    reviewAdminAdmissionsContribution,
    type AdmissionsContribution,
    type ContributionCategory,
    type ContributionStatus
} from "../../../lib/api/contributions";
import { ApiError } from "../../../lib/api/types";
import { useAdmin } from "../../admin-context";
import {
    AdminPanel,
    Badge,
    EmptyState,
    ErrorText,
    formatDate,
    LoadingState
} from "../../admin-ui";

const categoryLabels: Record<ContributionCategory, string> = {
    applicants: "應屆報名人數",
    second_stage: "應屆錄取二階人數",
    waitlist: "應屆遞補人數",
    historical_brochure: "歷屆簡章"
};

const statusLabels: Record<ContributionStatus, string> = {
    pending: "待審核",
    accepted: "已採用",
    rejected: "未採用"
};

export default function AdmissionsContributionsView() {
    const { mfaCode } = useAdmin();
    const [status, setStatus] = useState<ContributionStatus | "all">("pending");
    const [items, setItems] = useState<AdmissionsContribution[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [busyID, setBusyID] = useState<string | null>(null);
    const [activeAttachment, setActiveAttachment] = useState<{
        contributionID: string;
        attachmentID: string;
        url: string;
    } | null>(null);
    const attachmentURLRef = useRef<string | null>(null);

    useEffect(() => () => {
        if (attachmentURLRef.current) URL.revokeObjectURL(attachmentURLRef.current);
    }, []);

    const load = useCallback(async () => {
        setError(null);
        try {
            const response = await listAdminAdmissionsContributions(
                status === "all" ? undefined : status,
                mfaCode || undefined
            );
            setItems(response.data);
        } catch (cause) {
            setError(cause instanceof ApiError ? cause.message : "載入資料回報失敗。請稍後再試。");
            setItems((current) => current ?? []);
        }
    }, [mfaCode, status]);

    useEffect(() => {
        // eslint-disable-next-line react-hooks/set-state-in-effect
        void load();
    }, [load]);

    async function review(item: AdmissionsContribution, nextStatus: "accepted" | "rejected") {
        if (busyID) return;
        setBusyID(item.id);
        setError(null);
        try {
            await reviewAdminAdmissionsContribution(item.id, nextStatus, mfaCode || undefined);
            await load();
        } catch (cause) {
            setError(cause instanceof ApiError ? cause.message : "更新回報狀態失敗。請稍後再試。");
        } finally {
            setBusyID(null);
        }
    }

    async function loadAttachment(item: AdmissionsContribution, attachmentID: string) {
        if (busyID) return;
        setBusyID(item.id);
        setError(null);
        try {
            const blob = await getAdminAdmissionsContributionContent(item.id, attachmentID, mfaCode || undefined);
            const url = URL.createObjectURL(blob);
            if (attachmentURLRef.current) URL.revokeObjectURL(attachmentURLRef.current);
            attachmentURLRef.current = url;
            setActiveAttachment({ contributionID: item.id, attachmentID, url });
        } catch (cause) {
            setError(cause instanceof ApiError ? cause.message : "無法載入附件。請稍後再試。");
        } finally {
            setBusyID(null);
        }
    }

    async function downloadAttachment(item: AdmissionsContribution, attachmentID: string, filename: string) {
        if (busyID) return;
        setBusyID(item.id);
        setError(null);
        try {
            const blob = await getAdminAdmissionsContributionContent(item.id, attachmentID, mfaCode || undefined);
            const url = URL.createObjectURL(blob);
            const link = document.createElement("a");
            link.href = url;
            link.download = filename;
            document.body.append(link);
            link.click();
            link.remove();
            window.setTimeout(() => URL.revokeObjectURL(url), 60_000);
        } catch (cause) {
            setError(cause instanceof ApiError ? cause.message : "無法下載附件。請稍後再試。");
        } finally {
            setBusyID(null);
        }
    }

    return (
        <main className="article-dots -mx-5 -my-8 flex min-h-full flex-col border-y border-ink/5 px-5 py-10 sm:-mx-6 sm:-my-8 sm:px-6 lg:-mx-16 lg:-my-10 lg:px-16 lg:py-12">
            <div className="mx-auto w-full max-w-screen-xl">
                <header className="flex flex-wrap items-end justify-between gap-4">
                    <div>
                        <h1 className="font-serif text-3xl text-ink sm:text-4xl">資料回報</h1>
                        <p className="mt-2 max-w-2xl font-sans text-sm leading-6 text-copy-muted sm:text-base">
                            檢視使用者送出的招生人數與歷屆簡章資料。採用回報後，同意公開的暱稱會列入前台感謝名單。
                        </p>
                    </div>
                    <label className="flex items-center gap-2 font-sans text-sm font-medium text-ink">
                        篩選
                        <select
                            value={status}
                            onChange={(event) => setStatus(event.target.value as ContributionStatus | "all")}
                            className="h-10 rounded-[var(--radius-small)] border border-ink/15 bg-surface px-3 text-sm"
                        >
                            <option value="pending">待審核</option>
                            <option value="accepted">已採用</option>
                            <option value="rejected">未採用</option>
                            <option value="all">全部狀態</option>
                        </select>
                    </label>
                </header>

                {error ? (
                    <div className="mt-6 flex flex-wrap items-center justify-between gap-3">
                        <ErrorText>{error}</ErrorText>
                        <Button variant="secondary" className="h-9 px-4 text-sm" onClick={() => void load()}>
                            重新載入
                        </Button>
                    </div>
                ) : null}

                <section className="mt-6 flex flex-col gap-4" aria-label="資料回報清單">
                    {items === null ? (
                        <AdminPanel><LoadingState label="載入資料回報…" /></AdminPanel>
                    ) : items.length === 0 ? (
                        <AdminPanel>
                            <EmptyState label={error ? "載入失敗，請使用上方按鈕重新載入。" : "目前沒有符合條件的回報資料。"} />
                        </AdminPanel>
                    ) : (
                        items.map((item) => (
                            <ContributionCard
                                key={item.id}
                                item={item}
                                busy={busyID === item.id}
                                activeAttachment={activeAttachment?.contributionID === item.id ? activeAttachment : null}
                                onReview={review}
                                onLoadAttachment={loadAttachment}
                                onDownloadAttachment={downloadAttachment}
                            />
                        ))
                    )}
                </section>
            </div>
        </main>
    );
}

function ContributionCard({
    item,
    busy,
    activeAttachment,
    onReview,
    onLoadAttachment,
    onDownloadAttachment
}: {
    item: AdmissionsContribution;
    busy: boolean;
    activeAttachment: { contributionID: string; attachmentID: string; url: string } | null;
    onReview: (item: AdmissionsContribution, status: "accepted" | "rejected") => void;
    onLoadAttachment: (item: AdmissionsContribution, attachmentID: string) => void;
    onDownloadAttachment: (item: AdmissionsContribution, attachmentID: string, filename: string) => void;
}) {
    return (
        <AdminPanel className="p-5 sm:p-6">
            <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                        <h2 className="font-serif text-xl text-ink">
                            {item.school_name || categoryLabels[item.category]}
                        </h2>
                        <Badge tone={item.status === "accepted" ? "positive" : item.status === "rejected" ? "negative" : "warning"}>
                            {statusLabels[item.status]}
                        </Badge>
                    </div>
                    <p className="mt-1 font-sans text-sm text-copy-muted">
                        {categoryLabels[item.category]}・{item.academic_year} 學年度・{formatDate(item.created_at)}
                    </p>
                </div>
                {item.status === "pending" ? (
                    <div className="flex shrink-0 gap-2">
                        <Button
                            className="h-10 gap-2 px-4 text-sm"
                            disabled={busy}
                            onClick={() => onReview(item, "accepted")}
                        >
                            {busy ? <Loader2 aria-hidden className="size-4 animate-spin" /> : <Check aria-hidden className="size-4" />}
                            採用
                        </Button>
                        <Button
                            variant="secondary"
                            className="h-10 gap-2 px-4 text-sm"
                            disabled={busy}
                            onClick={() => onReview(item, "rejected")}
                        >
                            <X aria-hidden className="size-4" />
                            未採用
                        </Button>
                    </div>
                ) : null}
            </div>

            <dl className="mt-5 grid gap-4 border-t border-ink/10 pt-4 sm:grid-cols-2">
                <Detail label="聯絡信箱" value={item.email} />
                {item.school_name ? <Detail label="提供學校" value={item.school_name} /> : null}
                {item.nickname ? (
                    <Detail
                        label="暱稱"
                        value={`${item.nickname}${item.publish_nickname ? "（同意公開，採用後列入感謝名單）" : "（不公開）"}`}
                    />
                ) : null}
            </dl>

            {item.data_text ? (
                <section className="mt-5">
                    <h3 className="font-sans text-sm font-bold text-ink">回報內容</h3>
                    <pre className="mt-2 overflow-x-auto whitespace-pre-wrap rounded-[var(--radius-small)] bg-ink/[0.035] p-4 font-sans text-sm leading-6 text-ink">{item.data_text}</pre>
                </section>
            ) : null}

            {item.reference_url ? (
                <div className="mt-5 flex flex-wrap items-center gap-3">
                    <a
                        href={item.reference_url}
                        target="_blank"
                        rel="noreferrer"
                        className="inline-flex items-center gap-1.5 font-sans text-sm font-medium text-ink underline underline-offset-4"
                    >
                        開啟簡章連結 <ExternalLink aria-hidden className="size-4" />
                    </a>
                </div>
            ) : null}

            {item.attachments?.length ? (
                <section className="mt-5 border-t border-ink/10 pt-4">
                    <h3 className="font-sans text-sm font-bold text-ink">上傳檔案</h3>
                    <ul className="mt-3 grid gap-3">
                        {item.attachments.map((attachment) => {
                            const previewURL =
                                activeAttachment?.attachmentID === attachment.id ? activeAttachment.url : undefined;
                            const isPDF = attachment.mime_type === "application/pdf";
                            return (
                                <li key={attachment.id} className="rounded-[var(--radius-small)] border border-ink/10 bg-white/60 p-3">
                                    <div className="flex flex-wrap items-center justify-between gap-3">
                                        <div className="min-w-0">
                                            <p className="break-all font-sans text-sm font-medium text-ink">{attachment.file_name}</p>
                                            <p className="mt-1 font-sans text-xs text-copy-muted">
                                                {formatFileSize(attachment.size_bytes)}
                                            </p>
                                        </div>
                                        <div className="flex shrink-0 flex-wrap gap-2">
                                            {isPDF ? (
                                                <Button
                                                    variant="secondary"
                                                    className="h-9 gap-2 px-3 text-sm"
                                                    disabled={busy}
                                                    onClick={() => onLoadAttachment(item, attachment.id)}
                                                >
                                                    {busy ? <Loader2 aria-hidden className="size-4 animate-spin" /> : <Eye aria-hidden className="size-4" />}
                                                    {previewURL ? "重新載入預覽" : "站內預覽"}
                                                </Button>
                                            ) : null}
                                            <Button
                                                variant="secondary"
                                                className="h-9 gap-2 px-3 text-sm"
                                                disabled={busy}
                                                onClick={() => onDownloadAttachment(item, attachment.id, attachment.file_name)}
                                            >
                                                <Download aria-hidden className="size-4" />
                                                下載
                                            </Button>
                                        </div>
                                    </div>
                                    {isPDF && previewURL ? (
                                        <div className="mt-3 overflow-hidden rounded-[var(--radius-small)] border border-ink/10 bg-ink/[0.035]">
                                            <iframe
                                                title={`${attachment.file_name} PDF 預覽`}
                                                src={previewURL}
                                                referrerPolicy="no-referrer"
                                                loading="lazy"
                                                className="h-[70vh] min-h-[520px] w-full bg-white"
                                            />
                                        </div>
                                    ) : null}
                                </li>
                            );
                        })}
                    </ul>
                </section>
            ) : null}
        </AdminPanel>
    );
}

function Detail({ label, value }: { label: string; value: string }) {
    return (
        <div className="min-w-0">
            <dt className="font-sans text-xs font-bold tracking-wide text-copy-muted">{label}</dt>
            <dd className="mt-1 break-words font-sans text-sm text-ink">{value}</dd>
        </div>
    );
}

function formatFileSize(size: number): string {
    if (size < 1024 * 1024) return `${Math.max(1, Math.round(size / 1024))} KB`;
    return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}
