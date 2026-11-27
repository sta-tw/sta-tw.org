"use client";

import { useCallback, useState } from "react";
import Button from "../../components/button";
import { Input } from "../../components/input";
import Select from "../../components/select";
import { ErrorText, StatusText } from "../../components/status";
import {
    getApplicationResult,
    listApplicationInquiries,
    setCandidateNumber,
    setWillingness,
    WILLINGNESS_VALUES,
    willingnessLabels,
    type Application
} from "../../lib/api/applications";
import { ApiError } from "../../lib/api/types";
import { formatDateTime } from "../../lib/format";
import { useAction, useResource } from "../../lib/use-resource";

const willingnessOptions = [...WILLINGNESS_VALUES].reverse().map((value) => ({
    value: String(value),
    label: `${willingnessLabels[value]}（${value}）`
}));

const rowClass =
    "flex flex-col gap-4 rounded-[var(--radius-panel)] border border-ink/10 bg-surface p-4 sm:p-5";

const nameOf = (application: Application) =>
    `${application.school_name} ${application.program_name}`;

/** Application list and real backend inputs. Candidate numbers are write-only
 * in the current API, so the UI never invents a last-four value or preference rank. */
export default function PreferenceList({ applications }: { applications: Application[] }) {
    return (
        <ul aria-label="已登記報考校系" className="flex flex-col gap-3">
            {applications.map((application) => (
                <li key={application.id} className={rowClass}>
                    <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                        <div className="min-w-0">
                            <p className="font-sans text-sm text-copy-muted">
                                {application.academic_year} 學年度
                            </p>
                            <p className="font-sans text-lg leading-snug font-medium break-words text-ink">
                                {nameOf(application)}
                            </p>
                        </div>
                        <WillingnessControl applicationID={application.id} />
                    </div>
                    <CandidateNumberForm application={application} />
                </li>
            ))}
        </ul>
    );
}

function CandidateNumberForm({ application }: { application: Application }) {
    const [value, setValue] = useState("");
    const [candidateName, setCandidateName] = useState("");
    const [submitted, setSubmitted] = useState(false);
    const { pending, error, run } = useAction();
    const trimmed = value.trim();
    const trimmedName = candidateName.trim();

    async function submit(event: React.FormEvent) {
        event.preventDefault();
        setSubmitted(false);
        if (await run(() => setCandidateNumber(application.id, trimmed, trimmedName))) {
            setValue("");
            setCandidateName("");
            setSubmitted(true);
        }
    }

    return (
        <form onSubmit={submit} className="flex flex-col gap-2 border-t border-ink/10 pt-3">
            <div className="grid gap-3 sm:grid-cols-2">
                <div className="flex flex-col gap-1.5">
                    <label
                        className="font-sans text-sm font-medium text-ink"
                        htmlFor={`candidate-${application.id}`}
                    >
                        准考證號碼
                    </label>
                    <Input
                        id={`candidate-${application.id}`}
                        aria-describedby={`candidate-hint-${application.id}`}
                        value={value}
                        placeholder="輸入准考證號碼"
                        autoComplete="off"
                        required
                        minLength={4}
                        maxLength={64}
                        pattern="[A-Za-z0-9_\-]{4,64}"
                        onChange={(event) => {
                            setValue(event.target.value);
                            setSubmitted(false);
                        }}
                    />
                    <p
                        id={`candidate-hint-${application.id}`}
                        className="font-sans text-xs text-copy-muted"
                    >
                        請輸入 4–64 個英文字母、數字、連字號或底線。
                    </p>
                </div>
                <div className="flex flex-col gap-1.5">
                    <label
                        className="font-sans text-sm font-medium text-ink"
                        htmlFor={`candidate-name-${application.id}`}
                    >
                        考生姓名
                    </label>
                    <Input
                        id={`candidate-name-${application.id}`}
                        value={candidateName}
                        placeholder="輸入准考證上的姓名"
                        autoComplete="name"
                        required
                        maxLength={100}
                        onChange={(event) => {
                            setCandidateName(event.target.value);
                            setSubmitted(false);
                        }}
                    />
                </div>
            </div>
            <div className="flex flex-col gap-2 sm:flex-row">
                <Button
                    type="submit"
                    variant="secondary"
                    size="base"
                    className="shrink-0 px-5"
                    disabled={
                        pending !== null ||
                        trimmed.length < 4 ||
                        !trimmedName ||
                        trimmedName.length > 100
                    }
                >
                    {pending ? "送出中…" : "送出資料"}
                </Button>
            </div>
            <p className="font-sans text-sm text-copy-muted">
                准考證號碼與考生姓名會加密保存，供查榜核對；API 不會回傳這些資料。
            </p>
            <StatusText tone="success">
                {submitted ? "准考證號碼與考生姓名已送出。" : null}
            </StatusText>
            <ErrorText>{error}</ErrorText>
        </form>
    );
}

/** 404 means "not available yet" for both the result and the inquiries. */
const notFoundAs =
    <T,>(fallback: T) =>
    (cause: unknown): T => {
        if (cause instanceof ApiError && cause.status === 404) return fallback;
        throw cause;
    };

/** The backend opens willingness after the official result is available. */
function WillingnessControl({ applicationID }: { applicationID: string }) {
    const load = useCallback(
        async (signal: AbortSignal) => {
            const [result, inquiries] = await Promise.all([
                getApplicationResult(applicationID, { signal }).then(
                    ({ data }) => data,
                    notFoundAs(null)
                ),
                listApplicationInquiries(applicationID, { signal }).then(
                    ({ data }) => data,
                    notFoundAs([])
                )
            ]);
            return { result, inquiries };
        },
        [applicationID]
    );
    const { state, setData } = useResource(load);
    const { pending, error, run } = useAction();
    const [saved, setSaved] = useState(false);
    const ready = state.status === "ready" ? state.data : null;
    const canRespond =
        ready?.result?.result_status === "admitted" ||
        ready?.result?.result_status === "waitlisted";
    const openInquiry = ready?.inquiries
        .filter((inquiry) => !inquiry.responded)
        .sort((a, b) => b.created_at.localeCompare(a.created_at))[0];

    function change(next: string) {
        if (!ready?.result || next === "") return;
        const value = Number(next) as (typeof WILLINGNESS_VALUES)[number];
        setSaved(false);
        void run(async () => {
            await setWillingness(applicationID, { value, inquiry_id: openInquiry?.id });
            setData((data) => ({
                ...data,
                result: data.result && { ...data.result, current_willingness: value }
            }));
            setSaved(true);
        });
    }

    const current = ready?.result?.current_willingness;

    return (
        <div className="flex w-full flex-col gap-1 lg:w-56 lg:shrink-0">
            <Select
                aria-label="報到意願"
                value={current === undefined ? "" : String(current)}
                placeholder={
                    state.status === "loading"
                        ? "載入中…"
                        : canRespond
                          ? "請選擇報到意願"
                          : "放榜後開放填寫"
                }
                options={willingnessOptions}
                disabled={!canRespond || pending !== null}
                onChange={(event) => change(event.target.value)}
            />
            {openInquiry?.response_deadline && (
                <p className="font-sans text-xs text-copy-muted">
                    請於 {formatDateTime(openInquiry.response_deadline)} 前回覆
                </p>
            )}
            <StatusText>{saved ? "已更新報到意願。" : null}</StatusText>
            <ErrorText>{error}</ErrorText>
        </div>
    );
}
