"use client";

import { useCallback, useState } from "react";
import Button from "../../components/button";
import Field from "../../components/field";
import { Input } from "../../components/input";
import Select from "../../components/select";
import {
    getApplicationResult,
    listApplicationInquiries,
    setCandidateNumber,
    setWillingness,
    WILLINGNESS_VALUES,
    type ApplicationInquiry,
    type ApplicationResult,
    type ResultStatus
} from "../../lib/api/applications";
import { ApiError } from "../../lib/api/types";
import { Resource, StatusBadge, type BadgeTone } from "../../components/status";
import { useAction, useResource } from "../../lib/use-resource";
import { formatDateTime } from "../../lib/format";

/** 404 means "not available yet" for both the result and the inquiries. */
const notFoundAs =
    <T,>(fallback: T) =>
    (cause: unknown): T => {
        if (cause instanceof ApiError && cause.status === 404) return fallback;
        throw cause;
    };

const resultStatus: Record<ResultStatus, { label: string; tone: BadgeTone }> = {
    admitted: { label: "正取", tone: "success" },
    waitlisted: { label: "備取", tone: "pending" },
    rejected: { label: "未錄取", tone: "danger" },
    unknown: { label: "結果未明", tone: "neutral" }
};

const willingnessLabels: Record<(typeof WILLINGNESS_VALUES)[number], string> = {
    100: "100 — 確定就讀",
    80: "80 — 很可能就讀",
    60: "60 — 傾向就讀",
    40: "40 — 還在考慮",
    20: "20 — 不太可能就讀",
    0: "0 — 確定放棄"
};

const willingnessOptions = [...WILLINGNESS_VALUES].reverse().map((value) => ({
    value: String(value),
    label: willingnessLabels[value]
}));

const inquiryRounds: Record<ApplicationInquiry["round"], string> = {
    result_released: "榜單公布意願調查",
    acceptance_deadline: "正取截止前確認"
};

export default function ResultPanel({ applicationID }: { applicationID: string }) {
    const load = useCallback(
        async (signal: AbortSignal) => {
            const [result, inquiries] = await Promise.all([
                getApplicationResult(applicationID, { signal }).then(
                    ({ data }) => data,
                    notFoundAs<ApplicationResult | null>(null)
                ),
                listApplicationInquiries(applicationID, { signal }).then(
                    ({ data }) => data,
                    notFoundAs<ApplicationInquiry[]>([])
                )
            ]);
            return { result, inquiries };
        },
        [applicationID]
    );
    const { state, reload } = useResource(load);

    return (
        <Resource state={state} reload={reload} loadingLabel="載入查榜結果">
            {({ result, inquiries }) => (
                <ResultBody
                    result={result}
                    inquiries={inquiries}
                    applicationID={applicationID}
                    onSaved={reload}
                />
            )}
        </Resource>
    );
}

function ResultBody({
    applicationID,
    result,
    inquiries,
    onSaved
}: {
    applicationID: string;
    result: ApplicationResult | null;
    inquiries: ApplicationInquiry[];
    onSaved: () => void;
}) {
    const canRespond =
        result?.result_status === "admitted" || result?.result_status === "waitlisted";

    return (
        <div className="flex flex-col gap-6">
            {result ? (
                <ResultReport result={result} />
            ) : (
                <p className="font-sans text-base text-copy-muted">
                    榜單尚未公布或尚未比對到你的准考證號碼。先填好准考證號碼，公布後就會自動比對。
                </p>
            )}
            <CandidateNumberForm applicationID={applicationID} result={result} onSaved={onSaved} />
            {canRespond && (
                <WillingnessForm
                    applicationID={applicationID}
                    current={result.current_willingness}
                    inquiries={inquiries}
                    onSaved={onSaved}
                />
            )}
        </div>
    );
}

function ResultReport({ result }: { result: ApplicationResult }) {
    const status = resultStatus[result.result_status];
    const stats: { label: string; value: string; hint?: string }[] = [];
    if (result.official_rank !== undefined) {
        stats.push({ label: "官方名次", value: `${status.label} ${result.official_rank}` });
    }
    if (result.quota !== undefined) stats.push({ label: "招生名額", value: `${result.quota} 名` });
    if (result.result_status === "waitlisted") {
        stats.push({
            label: "前方考生",
            value: `${result.front_candidate_count} 人`,
            hint: `其中 ${result.front_response_count} 人已回覆意願`
        });
    }
    if (result.position_after_declines !== undefined) {
        stats.push({
            label: "扣除放棄後順位",
            value: `第 ${result.position_after_declines} 位`
        });
    }
    if (result.reference_probability !== undefined) {
        stats.push({
            label: "參考錄取機率",
            value: `${Math.round(result.reference_probability)}%`,
            hint: "依前方考生回覆的意願估算，僅供參考"
        });
    }

    return (
        <section aria-label="查榜結果" className="flex flex-col gap-4">
            <div className="flex items-center gap-3">
                <span className="font-sans text-base font-medium text-ink">榜單結果</span>
                <StatusBadge tone={status.tone}>{status.label}</StatusBadge>
            </div>
            {stats.length > 0 && (
                <dl className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                    {stats.map((stat) => (
                        <div
                            key={stat.label}
                            className="rounded-[var(--radius-control)] bg-ink/5 px-4 py-3"
                        >
                            <dt className="font-sans text-sm text-copy-muted">{stat.label}</dt>
                            <dd className="mt-1 font-sans text-2xl font-medium text-ink">
                                {stat.value}
                            </dd>
                            {stat.hint && (
                                <dd className="mt-1 font-sans text-sm text-copy-muted">
                                    {stat.hint}
                                </dd>
                            )}
                        </div>
                    ))}
                </dl>
            )}
        </section>
    );
}

function CandidateNumberForm({
    applicationID,
    result,
    onSaved
}: {
    applicationID: string;
    result: ApplicationResult | null;
    onSaved: () => void;
}) {
    const [value, setValue] = useState("");
    const { pending, error, run } = useAction();
    const saving = pending !== null;
    const current = result?.candidate_number_set ? result.candidate_number_last4 : undefined;

    async function submit(event: React.FormEvent) {
        event.preventDefault();
        const ok = await run(() => setCandidateNumber(applicationID, value.trim()));
        if (ok) {
            setValue("");
            onSaved();
        }
    }

    return (
        <form onSubmit={submit} className="flex flex-col gap-3 sm:flex-row sm:items-start">
            <Field
                label="准考證號碼"
                className="flex-1"
                hint={
                    current
                        ? `已設定，末四碼 ${current}。重新輸入即可更新。`
                        : "4–64 個英數字，可含 - 或 _；只會加密保存，頁面上僅顯示末四碼。"
                }
                error={error}
            >
                <Input
                    value={value}
                    autoComplete="off"
                    inputMode="text"
                    minLength={4}
                    maxLength={64}
                    pattern="[A-Za-z0-9_\-]{4,64}"
                    onChange={(event) => setValue(event.target.value)}
                />
            </Field>
            <Button
                type="submit"
                disabled={saving || value.trim().length < 4}
                size="base"
                className="sm:mt-8"
            >
                {saving ? "儲存中…" : current ? "更新" : "儲存"}
            </Button>
        </form>
    );
}

function WillingnessForm({
    applicationID,
    current,
    inquiries,
    onSaved
}: {
    applicationID: string;
    current?: number;
    inquiries: ApplicationInquiry[];
    onSaved: () => void;
}) {
    const [value, setValue] = useState(current === undefined ? "" : String(current));
    const { pending, error, run } = useAction();
    const saving = pending !== null;
    const openInquiry = inquiries
        .filter((inquiry) => !inquiry.responded)
        .sort((a, b) => b.created_at.localeCompare(a.created_at))[0];

    async function submit(event: React.FormEvent) {
        event.preventDefault();
        const ok = await run(() =>
            setWillingness(applicationID, {
                value: Number(value) as (typeof WILLINGNESS_VALUES)[number],
                inquiry_id: openInquiry?.id
            })
        );
        if (ok) onSaved();
    }

    return (
        <section aria-labelledby={`willingness-${applicationID}`} className="flex flex-col gap-4">
            <h4
                id={`willingness-${applicationID}`}
                className="font-sans text-base font-medium text-ink"
            >
                就讀意願
            </h4>
            {inquiries.length > 0 && (
                <ul className="flex flex-col gap-2">
                    {inquiries.map((inquiry) => (
                        <li
                            key={inquiry.id}
                            className="flex flex-wrap items-center gap-x-3 gap-y-1 font-sans text-sm text-copy-muted"
                        >
                            <span className="font-medium text-ink">
                                {inquiryRounds[inquiry.round] ?? inquiry.round}
                            </span>
                            {inquiry.response_deadline && (
                                <span>
                                    截止{" "}
                                    <time dateTime={inquiry.response_deadline}>
                                        {formatDateTime(inquiry.response_deadline)}
                                    </time>
                                </span>
                            )}
                            <StatusBadge tone={inquiry.responded ? "success" : "pending"}>
                                {inquiry.responded
                                    ? `已回覆 ${inquiry.current_willingness ?? ""}`.trim()
                                    : "待回覆"}
                            </StatusBadge>
                        </li>
                    ))}
                </ul>
            )}
            <form onSubmit={submit} className="flex flex-col gap-3 sm:flex-row sm:items-start">
                <Field
                    label="你的就讀意願"
                    className="flex-1"
                    hint="匿名彙整後用來估算備取考生的參考錄取機率，不會公開你的身分。"
                    error={error}
                >
                    <Select
                        value={value}
                        placeholder="請選擇"
                        options={willingnessOptions}
                        onChange={(event) => setValue(event.target.value)}
                    />
                </Field>
                <Button
                    type="submit"
                    disabled={saving || value === ""}
                    size="base"
                    className="sm:mt-8"
                >
                    {saving ? "送出中…" : "送出意願"}
                </Button>
            </form>
        </section>
    );
}
