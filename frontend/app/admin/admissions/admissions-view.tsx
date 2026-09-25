"use client";

import { useEffect, useRef, useState } from "react";
import { Dialog } from "radix-ui";
import { twMerge } from "tailwind-merge";
import {
    Check,
    ExternalLink,
    Eye,
    EyeOff,
    FileDown,
    History,
    Loader2,
    RefreshCw,
    Search,
    Upload,
    X
} from "lucide-react";
import Button from "../../components/button";
import {
    getAdminBrochureContentURL,
    getAdminBrochureDownload,
    getAdminBrochureUpload,
    getAdminBrochureUploadContentURL,
    listAdminBrochureEvents,
    listAdminBrochures,
    listAdminBrochureUploads,
    listAdminAdmissionProgramHistory,
    listAdminAdmissionPrograms,
    confirmAdminBrochureUpload,
    rejectAdminBrochureUpload,
    reviewAdminBrochure,
    reviewAdminAdmissionProgram,
    setAdminBrochureVisibility,
    syncAdminAdmissionPrograms,
    updateAdminAdmissionProgram,
    uploadAdminBrochure,
    type AdminAdmissionProgram,
    type AdmissionExamItem,
    type AdmissionProgramInput,
    type AdmissionTimelineEvent,
    type BrochureDocument,
    type BrochureEvent,
    type BrochureUpload,
    type BrochureUploadCandidate,
    type ProgramAuditEvent,
    type ProgramReviewStatus
} from "../../lib/api/admissions";
import { ApiError } from "../../lib/api/types";
import { useAdmin } from "../admin-context";

const inputClass =
    "rounded-[var(--radius-small)] border border-ink/15 bg-surface px-3 py-2 font-sans text-sm text-ink outline-none focus:border-ink/40";
const textareaClass =
    "min-h-28 w-full resize-y rounded-[var(--radius-small)] border border-ink/15 bg-surface px-3 py-2 font-mono text-xs leading-5 text-ink outline-none focus:border-ink/40";
const panelClass =
    "rounded-[var(--radius-panel)] border border-ink/10 bg-surface/80 p-5 shadow-sm sm:p-6";
const programPageSize = 50;

const programStatusLabel: Record<ProgramReviewStatus, string> = {
    draft: "草稿",
    pending: "待審核",
    approved: "已核准",
    published: "已上架",
    rejected: "已退回",
    archived: "已封存"
};

const brochureStatusLabel: Record<string, string> = {
    pending: "待審核",
    published: "已上架",
    rejected: "已退回",
    archived: "已下架"
};

const brochureUploadStatusLabel: Record<string, string> = {
    queued: "待處理",
    processing: "待處理",
    pending_review: "待審核",
    approved: "已完成",
    rejected: "已退回",
    failed: "待處理"
};

type UploadFilter = "all" | "pending_review" | "pending" | "rejected" | "approved";

const uploadFilterOptions: { value: UploadFilter; label: string }[] = [
    { value: "all", label: "全部" },
    { value: "pending_review", label: "待審核" },
    { value: "pending", label: "待處理" },
    { value: "rejected", label: "已退回" },
    { value: "approved", label: "已完成" }
];

// "待處理" covers three raw statuses (queued/processing/failed); the API's
// `status` filter only takes one value, so this filters the already-loaded
// list client-side instead of re-querying per tab.
const uploadFilterStatuses: Record<UploadFilter, BrochureUpload["status"][] | null> = {
    all: null,
    pending: ["queued", "processing", "failed"],
    pending_review: ["pending_review"],
    rejected: ["rejected"],
    approved: ["approved"]
};

const syncTemplate: AdmissionProgramInput = {
    academic_year: 115,
    school_code: "001",
    program_code: "023",
    admission_program_name: "特殊選材示範學系",
    admission_quota: 3,
    exam_items: [
        {
            name: "書面審查",
            stage: "初試",
            sort_order: 1,
            weight_percent: 60,
            description: "審查學習歷程與作品",
            source_page: "23"
        },
        {
            name: "面試",
            stage: "複試",
            sort_order: 2,
            weight_percent: 40,
            description: "評估專業興趣與學習動機",
            source_page: "24"
        }
    ],
    timeline_events: [
        {
            name: "網路報名",
            start_date: "2026-09-29",
            start_time: "09:00",
            end_date: "2026-10-07",
            end_time: "17:00",
            sort_order: 1,
            notes: "-"
        },
        {
            name: "錄取放榜",
            start_date: "2026-12-11",
            start_time: "10:00",
            end_date: "-",
            end_time: "-",
            sort_order: 2,
            notes: "-"
        }
    ],
    brochure_is_tentative: false,
    consultation_phone: "-",
    consultation_email: "-",
    consultation_contact: "-",
    brochure_url: "-",
    special_talent_target: "-",
    different_education_backgrounds: "-",
    different_education_other: "-",
    notes: "-",
    source_page: 23,
    registration_fee: "-",
    exam_location: "-",
    recommendation_letter_deadline: "-",
    portfolio_deadline: "-",
    checkin_waitlist_process: "-",
    fee_reduction_eligibility: "-",
    admission_group: "-",
    cross_group: "-",
    admission_category: "-",
    priority_admission: "-",
    portfolio_required: "-",
    recommendation_letter_type: "-",
    max_applicable_programs: "-",
    applicant_count: "-",
    interview_count: "-",
    admitted_count: "-",
    waitlisted_count: "-",
    admission_rate: "-",
    first_stage_pass_rate: "-",
    competition_ratio: "-",
    school_official_url: "-",
    department_official_url: "-"
};

function describeError(cause: unknown): string {
    if (cause instanceof ApiError) return cause.message || `發生錯誤（${cause.code}）`;
    return "發生未知錯誤，請稍後再試。";
}

export default function AdmissionsView() {
    const { mfaCode } = useAdmin();
    const [activeTab, setActiveTab] = useState<"programs" | "brochures">("programs");

    const [programYear, setProgramYear] = useState("");
    const [programSchool, setProgramSchool] = useState("");
    const [programStatus, setProgramStatus] = useState<ProgramReviewStatus | "">("");
    const [programQuery, setProgramQuery] = useState("");
    const [programs, setPrograms] = useState<AdminAdmissionProgram[] | null>(null);
    const [programLoading, setProgramLoading] = useState(false);
    const [programError, setProgramError] = useState<string | null>(null);
    const [programHasMore, setProgramHasMore] = useState(false);
    const [selectedProgram, setSelectedProgram] = useState<AdminAdmissionProgram | null>(null);
    const [syncOpen, setSyncOpen] = useState(false);

    const [brochures, setBrochures] = useState<BrochureDocument[] | null>(null);
    const [brochureLoading, setBrochureLoading] = useState(false);
    const [brochureError, setBrochureError] = useState<string | null>(null);
    const [selectedBrochure, setSelectedBrochure] = useState<BrochureDocument | null>(null);

    const [brochureUploads, setBrochureUploads] = useState<BrochureUpload[] | null>(null);
    const [brochureUploadLoading, setBrochureUploadLoading] = useState(false);
    const [brochureUploadError, setBrochureUploadError] = useState<string | null>(null);
    const [selectedBrochureUpload, setSelectedBrochureUpload] = useState<BrochureUpload | null>(
        null
    );
    const [uploadFilter, setUploadFilter] = useState<UploadFilter>("all");
    const [uploading, setUploading] = useState(false);
    const [uploadError, setUploadError] = useState<string | null>(null);
    const [uploadNotice, setUploadNotice] = useState<string | null>(null);
    const fileInputRef = useRef<HTMLInputElement>(null);

    async function loadPrograms(offset = 0, append = false) {
        setProgramLoading(true);
        setProgramError(null);
        try {
            const response = await listAdminAdmissionPrograms(
                {
                    academicYear: parseYear(programYear),
                    schoolCode: programSchool.trim() || undefined,
                    reviewStatus: programStatus || undefined,
                    q: programQuery.trim() || undefined,
                    limit: programPageSize,
                    offset
                },
                mfaCode || undefined
            );
            setPrograms((previous) =>
                append && previous ? [...previous, ...response.data] : response.data
            );
            setProgramHasMore(response.data.length === programPageSize);
        } catch (cause) {
            setProgramError(describeError(cause));
        } finally {
            setProgramLoading(false);
        }
    }

    async function loadBrochures() {
        setBrochureLoading(true);
        setBrochureError(null);
        try {
            const response = await listAdminBrochures(undefined, mfaCode || undefined);
            setBrochures(response.data);
        } catch (cause) {
            setBrochureError(describeError(cause));
        } finally {
            setBrochureLoading(false);
        }
    }

    async function loadBrochureUploads() {
        setBrochureUploadLoading(true);
        setBrochureUploadError(null);
        try {
            const response = await listAdminBrochureUploads(undefined, mfaCode || undefined);
            setBrochureUploads(response.data);
        } catch (cause) {
            setBrochureUploadError(describeError(cause));
        } finally {
            setBrochureUploadLoading(false);
        }
    }

    useEffect(() => {
        // The admin shell has already completed the role/MFA gate before this
        // page renders. These calls are therefore safe to make on entry and
        // when the verified MFA grant changes.
        // eslint-disable-next-line react-hooks/set-state-in-effect
        void Promise.all([loadPrograms(), loadBrochures(), loadBrochureUploads()]);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [mfaCode]);

    useEffect(() => {
        if (activeTab !== "brochures") return;

        const timer = window.setInterval(() => {
            void Promise.all([loadBrochures(), loadBrochureUploads()]);
        }, 10000);

        return () => window.clearInterval(timer);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [activeTab, mfaCode]);

    function handleProgramFilterSubmit(event: React.FormEvent) {
        event.preventDefault();
        const error = validateYearAndSchool(programYear, programSchool);
        if (error) {
            setProgramError(error);
            return;
        }
        void loadPrograms();
    }

    async function handleUploadFile(file: File) {
        setUploadError(null);
        setUploadNotice(null);

        if (!file.name.toLowerCase().endsWith(".pdf")) {
            setUploadError("只接受 PDF 檔案。");
            return;
        }

        const formData = new FormData();
        formData.append("file", file);

        setUploading(true);
        try {
            const response = await uploadAdminBrochure(formData, mfaCode || undefined);
            const detail = await getAdminBrochureUpload(
                response.data.upload_id,
                mfaCode || undefined
            );
            setSelectedBrochureUpload(detail.data);
            setUploadNotice("簡章已上傳，請確認 PDF 與建檔欄位後建立簡章。");
            if (fileInputRef.current) fileInputRef.current.value = "";
            await loadBrochureUploads();
        } catch (cause) {
            setUploadError(describeError(cause));
        } finally {
            setUploading(false);
        }
    }

    function handleUploadFileSelected(event: React.ChangeEvent<HTMLInputElement>) {
        const file = event.target.files?.[0];
        if (file) void handleUploadFile(file);
    }

    function replaceProgram(updated: AdminAdmissionProgram) {
        setPrograms(
            (previous) =>
                previous?.map((item) =>
                    item.program_identifier === updated.program_identifier ? updated : item
                ) ?? previous
        );
        setSelectedProgram(updated);
    }

    function replaceBrochure(updated: BrochureDocument) {
        setBrochures(
            (previous) =>
                previous?.map((item) =>
                    item.academic_year === updated.academic_year &&
                    item.school_code === updated.school_code
                        ? updated
                        : item
                ) ?? previous
        );
        setSelectedBrochure(updated);
    }

    function replaceBrochureUpload(updated: BrochureUpload) {
        setBrochureUploads(
            (previous) =>
                previous?.map((item) => (item.id === updated.id ? updated : item)) ?? previous
        );
        setSelectedBrochureUpload(updated);
    }

    function handleBrochureUploadConfirmed() {
        setSelectedBrochureUpload(null);
        void Promise.all([loadPrograms(), loadBrochures(), loadBrochureUploads()]);
    }

    const pendingPrograms =
        programs?.filter((item) => item.review_status === "pending").length ?? 0;
    const pendingBrochures =
        brochures?.filter((item) => item.review_status === "pending").length ?? 0;
    const pendingBrochureUploads =
        brochureUploads?.filter((item) => item.status === "pending_review").length ?? 0;
    const allowedUploadStatuses = uploadFilterStatuses[uploadFilter];
    const filteredBrochureUploads = allowedUploadStatuses
        ? (brochureUploads?.filter((item) => allowedUploadStatuses.includes(item.status)) ?? null)
        : brochureUploads;

    return (
        <div className="article-dots -mx-5 -my-8 flex min-h-full flex-col border-y border-ink/5 px-5 py-10 sm:-mx-6 sm:-my-8 sm:px-6 lg:-mx-16 lg:-my-10 lg:px-16 lg:py-12">
            <div className="mx-auto flex w-full max-w-screen-xl flex-col gap-8">
                <div className="flex flex-wrap items-end justify-between gap-4 border-b border-ink/10 pb-8">
                    <div>
                        <p className="font-sans text-sm text-ink/60">管理後台・招生資料</p>
                        <h1 className="mt-2 font-serif text-4xl tracking-[-0.04em] text-ink sm:text-5xl lg:text-6xl">
                            簡章管理
                        </h1>
                        <p className="mt-3 max-w-2xl font-sans text-sm leading-6 text-ink/60 sm:text-base">
                            管理員只需上傳 PDF，確認系統讀取的建檔資料後即可建立並發布。
                        </p>
                    </div>
                    <div className="flex gap-2 font-sans text-xs text-copy-muted">
                        <span className="rounded-full bg-accent-yellow/80 px-3 py-1.5">
                            校系待審 {pendingPrograms}
                        </span>
                        <span className="rounded-full bg-accent-green/55 px-3 py-1.5">
                            API 簡章待複核 {pendingBrochureUploads}
                        </span>
                        <span className="rounded-full bg-ink/10 px-3 py-1.5">
                            已建檔待審 {pendingBrochures}
                        </span>
                    </div>
                </div>

                <div className="flex max-w-full gap-7 overflow-x-auto border-b border-ink/10">
                    <TabButton
                        active={activeTab === "programs"}
                        onClick={() => setActiveTab("programs")}
                    >
                        招生資料
                    </TabButton>
                    <TabButton
                        active={activeTab === "brochures"}
                        onClick={() => setActiveTab("brochures")}
                    >
                        簡章檔案
                    </TabButton>
                </div>

                {activeTab === "programs" ? (
                    <section className="flex flex-col gap-4">
                        <div className={panelClass}>
                            <div className="flex flex-wrap items-start justify-between gap-4">
                                <div>
                                    <h2 className="font-serif text-xl text-ink">招生科系資料</h2>
                                    <p className="mt-1 font-sans text-sm text-copy-muted">
                                        檢查同步進來的校系資料，審核通過後才會供前台搜尋。
                                    </p>
                                </div>
                                <Button
                                    type="button"
                                    onClick={() => setSyncOpen(true)}
                                    className="h-10 gap-2 px-4 text-sm"
                                >
                                    <RefreshCw aria-hidden className="h-4 w-4" />
                                    批次同步
                                </Button>
                            </div>

                            <form
                                onSubmit={handleProgramFilterSubmit}
                                className="mt-5 flex flex-wrap items-end gap-3 border-t border-ink/10 pt-5"
                            >
                                <Field label="學年度">
                                    <input
                                        className={inputClass}
                                        inputMode="numeric"
                                        maxLength={3}
                                        placeholder="例如 115"
                                        value={programYear}
                                        onChange={(event) =>
                                            setProgramYear(event.target.value.replace(/\D/g, ""))
                                        }
                                    />
                                </Field>
                                <Field label="學校編號">
                                    <input
                                        className={inputClass}
                                        inputMode="numeric"
                                        maxLength={3}
                                        placeholder="例如 001"
                                        value={programSchool}
                                        onChange={(event) =>
                                            setProgramSchool(event.target.value.replace(/\D/g, ""))
                                        }
                                    />
                                </Field>
                                <Field label="審核狀態">
                                    <select
                                        className={inputClass}
                                        value={programStatus}
                                        onChange={(event) =>
                                            setProgramStatus(
                                                event.target.value as ProgramReviewStatus | ""
                                            )
                                        }
                                    >
                                        <option value="">全部狀態</option>
                                        <option value="pending">待審核</option>
                                        <option value="published">已上架</option>
                                        <option value="rejected">已退回</option>
                                        <option value="archived">已封存</option>
                                    </select>
                                </Field>
                                <Field label="搜尋校名／科系／識別碼">
                                    <input
                                        className={`${inputClass} w-60 max-w-full`}
                                        placeholder="輸入關鍵字"
                                        value={programQuery}
                                        onChange={(event) => setProgramQuery(event.target.value)}
                                    />
                                </Field>
                                <Button
                                    type="submit"
                                    disabled={programLoading}
                                    className="h-10 gap-2 px-5 text-sm"
                                >
                                    <Search aria-hidden className="h-4 w-4" />
                                    篩選
                                </Button>
                                <Button
                                    type="button"
                                    onClick={() => void loadPrograms()}
                                    disabled={programLoading}
                                    className="h-10 border border-ink/15 bg-surface px-4 text-sm text-ink hover:bg-ink/5"
                                >
                                    重新整理
                                </Button>
                            </form>
                        </div>

                        {programError ? (
                            <p className="font-sans text-sm text-red-600">{programError}</p>
                        ) : null}

                        <div className="overflow-x-auto rounded-[var(--radius-panel)] bg-surface shadow-[var(--shadow-card)]">
                            <table className="w-full min-w-[900px] font-sans text-sm">
                                <thead>
                                    <tr className="border-b border-ink/10 text-left text-copy-muted">
                                        <Th>識別碼</Th>
                                        <Th>學校／科系</Th>
                                        <Th>名額</Th>
                                        <Th>狀態</Th>
                                        <Th>更新時間</Th>
                                        <Th />
                                    </tr>
                                </thead>
                                <tbody>
                                    {programs?.map((program) => (
                                        <tr
                                            key={program.program_identifier}
                                            className="border-b border-ink/5 last:border-0"
                                        >
                                            <td className="px-4 py-3 font-mono text-xs text-copy-muted">
                                                {program.program_identifier}
                                            </td>
                                            <td className="px-4 py-3">
                                                <p className="font-bold text-ink">
                                                    {program.admission_program_name}
                                                </p>
                                                <p className="mt-1 text-xs text-copy-muted">
                                                    {program.school_name}（{program.school_code}）
                                                </p>
                                            </td>
                                            <td className="px-4 py-3 text-ink">
                                                {program.admission_quota}
                                            </td>
                                            <td className="px-4 py-3">
                                                <StatusBadge
                                                    label={
                                                        programStatusLabel[program.review_status] ??
                                                        program.review_status
                                                    }
                                                    status={program.review_status}
                                                />
                                            </td>
                                            <td className="px-4 py-3 text-xs whitespace-nowrap text-copy-muted">
                                                {formatDate(program.updated_at)}
                                            </td>
                                            <td className="px-4 py-3 text-right">
                                                <button
                                                    type="button"
                                                    onClick={() => setSelectedProgram(program)}
                                                    className="font-sans text-xs font-bold text-ink underline underline-offset-2"
                                                >
                                                    檢視／審核
                                                </button>
                                            </td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                            {programs === null ? (
                                <LoadingState />
                            ) : programs.length === 0 ? (
                                <EmptyState text="沒有符合條件的招生資料。" />
                            ) : null}
                        </div>

                        {programHasMore ? (
                            <Button
                                type="button"
                                onClick={() => void loadPrograms(programs?.length ?? 0, true)}
                                disabled={programLoading}
                                className="w-fit self-center border border-ink/15 bg-surface px-6 text-sm text-ink hover:bg-ink/5"
                            >
                                {programLoading ? "載入中…" : "載入更多"}
                            </Button>
                        ) : null}
                    </section>
                ) : (
                    <section className="flex flex-col gap-4">
                        <section className={panelClass}>
                            <div className="flex flex-wrap items-start justify-between gap-4">
                                <div>
                                    <h2 className="font-serif text-xl text-ink">簡章管理</h2>
                                    <p className="mt-1 max-w-3xl font-sans text-sm leading-6 text-copy-muted">
                                        上傳 PDF 後，系統會自動整理文字或執行
                                        OCR；確認建檔資料後即可建立並發布。
                                        <br />
                                        外部 AI／搜尋服務透過簡章 API
                                        送回的資料，會在下方等待人工複核。
                                    </p>
                                </div>
                                <div>
                                    <input
                                        ref={fileInputRef}
                                        className="sr-only"
                                        type="file"
                                        accept="application/pdf,.pdf"
                                        onChange={handleUploadFileSelected}
                                    />
                                    <Button
                                        type="button"
                                        disabled={uploading}
                                        onClick={() => fileInputRef.current?.click()}
                                        className="h-10 gap-2 px-5 text-sm"
                                    >
                                        {uploading ? (
                                            <Loader2 aria-hidden className="h-4 w-4 animate-spin" />
                                        ) : (
                                            <Upload aria-hidden className="h-4 w-4" />
                                        )}
                                        {uploading ? "上傳中…" : "上傳簡章PDF"}
                                    </Button>
                                </div>
                            </div>

                            {uploadError ? (
                                <p className="mt-4 font-sans text-sm text-red-600">{uploadError}</p>
                            ) : null}
                            {uploadNotice ? (
                                <p className="mt-4 font-sans text-sm text-ink/75">{uploadNotice}</p>
                            ) : null}

                            <div className="mt-6 flex flex-wrap gap-2 border-t border-ink/10 pt-6">
                                {uploadFilterOptions.map((option) => (
                                    <button
                                        key={option.value}
                                        type="button"
                                        onClick={() => setUploadFilter(option.value)}
                                        className={twMerge(
                                            "rounded-full px-3 py-1.5 font-sans text-xs font-bold transition-colors",
                                            uploadFilter === option.value
                                                ? "bg-ink text-surface"
                                                : "bg-ink/10 text-copy-muted hover:bg-ink/15"
                                        )}
                                    >
                                        {option.label}
                                    </button>
                                ))}
                            </div>

                            <BrochureUploadQueue
                                uploads={filteredBrochureUploads}
                                error={brochureUploadError}
                                onSelect={setSelectedBrochureUpload}
                                onReupload={() => fileInputRef.current?.click()}
                            />
                        </section>
                    </section>
                )}

                <ProgramDialog
                    key={`program-dialog-${selectedProgram?.program_identifier ?? "closed"}`}
                    program={selectedProgram}
                    mfaCode={mfaCode}
                    onClose={() => setSelectedProgram(null)}
                    onChanged={replaceProgram}
                />
                <BrochureDialog
                    key={`brochure-dialog-${
                        selectedBrochure
                            ? `${selectedBrochure.academic_year}-${selectedBrochure.school_code}`
                            : "closed"
                    }`}
                    brochure={selectedBrochure}
                    mfaCode={mfaCode}
                    onClose={() => setSelectedBrochure(null)}
                    onChanged={replaceBrochure}
                />
                <BrochureUploadReviewDialog
                    key={`brochure-upload-review-${selectedBrochureUpload?.id ?? "closed"}`}
                    upload={selectedBrochureUpload}
                    mfaCode={mfaCode}
                    onClose={() => setSelectedBrochureUpload(null)}
                    onChanged={replaceBrochureUpload}
                    onConfirmed={handleBrochureUploadConfirmed}
                />
                <SyncDialog
                    open={syncOpen}
                    mfaCode={mfaCode}
                    onClose={() => setSyncOpen(false)}
                    onSynced={() => void loadPrograms()}
                />
            </div>
        </div>
    );
}

function TabButton({
    active,
    onClick,
    children
}: {
    active: boolean;
    onClick: () => void;
    children: React.ReactNode;
}) {
    return (
        <button
            type="button"
            onClick={onClick}
            className={twMerge(
                "border-b-2 border-transparent px-1 pb-3 font-serif text-xl transition-colors sm:text-2xl",
                active
                    ? "border-[#f6bd42] text-ink"
                    : "text-ink/55 hover:border-ink/20 hover:text-ink"
            )}
        >
            {children}
        </button>
    );
}

function BrochureUploadQueue({
    uploads,
    error,
    onSelect,
    onReupload
}: {
    uploads: BrochureUpload[] | null;
    error: string | null;
    onSelect: (upload: BrochureUpload) => void;
    onReupload: () => void;
}) {
    return (
        <div className="mt-4">
            {error ? <p className="mt-4 font-sans text-sm text-red-600">{error}</p> : null}

            <div className="mt-4 overflow-x-auto rounded-[var(--radius-small)] border border-ink/10">
                <table className="w-full min-w-[780px] font-sans text-sm">
                    <thead>
                        <tr className="border-b border-ink/10 text-left text-copy-muted">
                            <Th>檔案</Th>
                            <Th>系統讀取結果</Th>
                            <Th>狀態</Th>
                            <Th>上傳時間</Th>
                            <Th />
                        </tr>
                    </thead>
                    <tbody>
                        {uploads?.map((upload) => (
                            <tr
                                key={`brochure-upload-${upload.id}`}
                                className="border-b border-ink/5 last:border-0"
                            >
                                <td className="px-4 py-3">
                                    <p className="max-w-64 truncate font-bold text-ink">
                                        {upload.original_file_name}
                                    </p>
                                    <p className="mt-1 font-mono text-[11px] text-copy-muted">
                                        SHA {upload.sha256.slice(0, 16)}… ·{" "}
                                        {formatBytes(upload.file_size_bytes)}
                                    </p>
                                </td>
                                <td className="px-4 py-3 text-xs text-copy-muted">
                                    <p>
                                        {upload.detected_academic_year
                                            ? `${upload.detected_academic_year} 學年度`
                                            : "學年度待確認"}
                                        {" · "}
                                        {detectedText(upload.detected_school_code) || "編號待確認"}
                                    </p>
                                    <p className="mt-1 max-w-72 truncate text-ink/70">
                                        {detectedText(upload.detected_school_name) || "校名待確認"}
                                    </p>
                                </td>
                                <td className="px-4 py-3">
                                    <UploadReportDialog upload={upload} />
                                </td>
                                <td className="px-4 py-3 text-xs whitespace-nowrap text-copy-muted">
                                    {formatDate(upload.created_at)}
                                </td>
                                <td className="px-4 py-3 text-right">
                                    {upload.status === "pending_review" ? (
                                        <button
                                            type="button"
                                            onClick={() => onSelect(upload)}
                                            className="font-sans text-xs font-bold text-ink underline underline-offset-2"
                                        >
                                            開始複核
                                        </button>
                                    ) : upload.status === "queued" ||
                                      upload.status === "processing" ? (
                                        <span className="text-xs text-copy-muted">等待完成</span>
                                    ) : upload.status === "failed" ? (
                                        <button
                                            type="button"
                                            onClick={onReupload}
                                            className="font-sans text-xs font-bold text-ink underline underline-offset-2"
                                        >
                                            重新上傳
                                        </button>
                                    ) : (
                                        <button
                                            type="button"
                                            onClick={() => onSelect(upload)}
                                            className="font-sans text-xs font-bold text-ink underline underline-offset-2"
                                        >
                                            已完成
                                        </button>
                                    )}
                                </td>
                            </tr>
                        ))}
                    </tbody>
                </table>
                {uploads === null ? (
                    <LoadingState />
                ) : uploads.length === 0 ? (
                    <EmptyState text="目前沒有 PDF 上傳紀錄。" />
                ) : null}
            </div>
        </div>
    );
}

function BrochureUploadReviewDialog({
    upload,
    mfaCode,
    onClose,
    onChanged,
    onConfirmed
}: {
    upload: BrochureUpload | null;
    mfaCode: string;
    onClose: () => void;
    onChanged: (upload: BrochureUpload) => void;
    onConfirmed: () => void;
}) {
    const [detail, setDetail] = useState<BrochureUpload | null>(null);
    const [academicYear, setAcademicYear] = useState("");
    const [schoolCode, setSchoolCode] = useState("");
    const [programs, setPrograms] = useState<AdmissionProgramInput[]>([]);
    const [selectedProgramIndex, setSelectedProgramIndex] = useState(0);
    const [reason, setReason] = useState("人工確認簡章資料");
    const [pdfURL, setPdfURL] = useState<string | null>(null);
    const [pdfLoading, setPdfLoading] = useState(false);
    const [pendingAction, setPendingAction] = useState<"confirm" | "reject" | "download" | null>(
        null
    );
    const [error, setError] = useState<string | null>(null);
    const initializedUploadID = useRef<string | null>(null);

    useEffect(() => {
        if (!upload) return;
        const currentUpload = upload;
        let ignore = false;
        let retryTimer: ReturnType<typeof setTimeout> | undefined;
        setDetail(null);
        setError(null);
        setPdfURL(null);
        setPdfLoading(true);
        initializedUploadID.current = null;
        setAcademicYear(
            currentUpload.detected_academic_year ? String(currentUpload.detected_academic_year) : ""
        );
        setSchoolCode(detectedText(currentUpload.detected_school_code));
        setPrograms([]);
        setSelectedProgramIndex(0);
        setReason("人工確認簡章資料");

        let objectURL: string | null = null;
        void fetch(getAdminBrochureUploadContentURL(currentUpload.id), {
            credentials: "include",
            headers: { Accept: "application/pdf" }
        })
            .then(async (response) => {
                if (!response.ok) {
                    let message = "PDF 預覽載入失敗。";
                    try {
                        const body = (await response.json()) as {
                            error?: { code?: string; message?: string };
                        };
                        message = body.error?.message || message;
                    } catch {
                        // Keep the friendly fallback when the error is not JSON.
                    }
                    throw new ApiError(response.status, "pdf_preview_failed", message);
                }
                const blob = await response.blob();
                objectURL = URL.createObjectURL(blob);
                if (!ignore) {
                    setPdfURL(objectURL);
                } else {
                    URL.revokeObjectURL(objectURL);
                }
            })
            .catch((cause) => {
                if (!ignore) setError(describeError(cause));
            })
            .finally(() => {
                if (!ignore) setPdfLoading(false);
            });

        async function loadDetail() {
            try {
                const response = await getAdminBrochureUpload(
                    currentUpload.id,
                    mfaCode || undefined
                );
                if (ignore) return;
                const next = response.data;
                setDetail(next);
                setAcademicYear(
                    next.detected_academic_year ? String(next.detected_academic_year) : ""
                );
                setSchoolCode(detectedText(next.detected_school_code));
                if (
                    initializedUploadID.current !== currentUpload.id &&
                    next.status === "pending_review"
                ) {
                    setPrograms(
                        (next.candidates ?? []).map((candidate) =>
                            candidateToProgramInput(candidate)
                        )
                    );
                    setSelectedProgramIndex(0);
                    initializedUploadID.current = currentUpload.id;
                }
                if (next.status === "queued" || next.status === "processing") {
                    retryTimer = setTimeout(() => void loadDetail(), 1500);
                }
            } catch (cause) {
                if (!ignore) setError(describeError(cause));
            }
        }

        void loadDetail();
        return () => {
            ignore = true;
            if (retryTimer) clearTimeout(retryTimer);
            if (objectURL) URL.revokeObjectURL(objectURL);
        };
    }, [mfaCode, upload]);

    async function download() {
        if (!upload) return;
        setPendingAction("download");
        setError(null);
        try {
            const url = pdfURL ?? getAdminBrochureUploadContentURL(upload.id);
            setPdfURL(url);
            window.open(url, "_blank", "noopener,noreferrer");
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setPendingAction(null);
        }
    }

    function updateProgram(index: number, updated: AdmissionProgramInput) {
        setPrograms((previous) =>
            previous.map((program, programIndex) => (programIndex === index ? updated : program))
        );
    }

    function addProgram() {
        const nextIndex = programs.length;
        setPrograms((previous) => [
            ...previous,
            emptyProgramInput(Number(academicYear) || 0, schoolCode)
        ]);
        setSelectedProgramIndex(nextIndex);
    }

    function removeProgram(index: number) {
        setPrograms((previous) => previous.filter((_, programIndex) => programIndex !== index));
        setSelectedProgramIndex((previous) => {
            if (previous > index) return previous - 1;
            return Math.min(previous, Math.max(0, programs.length - 2));
        });
    }

    async function confirm() {
        if (!upload) return;
        const year = Number(academicYear.trim());
        const code = schoolCode.trim();
        if (!/^\d{3}$/.test(academicYear.trim()) || year < 100 || year > 999) {
            setError("請確認學年度是 100～999 的三位數。 ");
            return;
        }
        if (!/^\d{3}$/.test(code) || code === "000") {
            setError("請確認學校編號是有效的三位數。 ");
            return;
        }
        if (programs.length === 0 || programs.length > 500) {
            setError("請至少建立一筆校系資料，最多 500 筆。 ");
            return;
        }
        const normalizedPrograms = programs.map((program) => ({
            ...program,
            academic_year: year,
            school_code: code
        }));
        const incompleteIndex = normalizedPrograms.findIndex(
            (program) =>
                !/^\d{3}$/.test(program.program_code.trim()) ||
                !program.admission_program_name.trim() ||
                program.exam_items.length === 0
        );
        if (incompleteIndex >= 0) {
            setError(
                `第 ${incompleteIndex + 1} 筆校系資料尚未完成：請填寫三位數系所編號、校系名稱，並至少新增一個考試項目。`
            );
            setSelectedProgramIndex(incompleteIndex);
            return;
        }
        setPendingAction("confirm");
        setError(null);
        try {
            const response = await confirmAdminBrochureUpload(
                upload.id,
                {
                    academic_year: year,
                    school_code: code,
                    programs: normalizedPrograms,
                    reason: reason.trim() || "人工確認簡章資料"
                },
                mfaCode || undefined
            );
            onChanged(response.data.upload);
            onConfirmed();
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setPendingAction(null);
        }
    }

    async function reject() {
        if (!upload) return;
        if (!reason.trim()) {
            setError("退回時請填寫原因。 ");
            return;
        }
        setPendingAction("reject");
        setError(null);
        try {
            const response = await rejectAdminBrochureUpload(
                upload.id,
                reason.trim(),
                mfaCode || undefined
            );
            onChanged(response.data);
            onClose();
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setPendingAction(null);
        }
    }

    const isExternalSubmission = upload?.intake_channel === "external_api";
    const effectiveStatus = detail?.status ?? upload?.status;
    const selectedProgram = programs[selectedProgramIndex] ?? null;
    const selectedPdfPage =
        selectedProgram?.source_page && selectedProgram.source_page > 0
            ? Math.floor(selectedProgram.source_page)
            : undefined;
    // navpanes=0 drops the thumbnail sidebar and zoom=page-fit scales the page
    // to the viewer's width, so the preview fills the panel instead of needing
    // a horizontal scroll to see the rest of the page.
    const pdfViewerParams = "navpanes=0&zoom=page-fit";
    const pdfPreviewSource = pdfURL
        ? `${pdfURL}#${selectedPdfPage ? `page=${selectedPdfPage}&` : ""}${pdfViewerParams}`
        : pdfURL;

    return (
        <Dialog.Root open={upload !== null} onOpenChange={(open) => !open && onClose()}>
            <Dialog.Portal>
                <Dialog.Overlay className="fixed inset-0 z-[80] bg-ink/40" />
                <Dialog.Content className="fixed top-1/2 left-1/2 z-[90] max-h-[calc(100vh-2rem)] w-[calc(100vw-2rem)] max-w-7xl -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-[var(--radius-panel)] bg-surface p-5 shadow-[var(--shadow-card)] sm:p-8">
                    <div className="flex items-start justify-between gap-4">
                        <div>
                            <Dialog.Title className="font-serif text-2xl text-ink">
                                {isExternalSubmission ? "人工複核 API 簡章" : "確認簡章建檔資料"}
                            </Dialog.Title>
                            <Dialog.Description className="mt-2 max-w-3xl font-sans text-sm leading-6 text-copy-muted">
                                {isExternalSubmission
                                    ? "這份資料由外部簡章 API 送回；請對照原始 PDF 修正擷取結果，確認後才會審核並上架。"
                                    : "請對照原始 PDF 完成下方建檔欄位。按下確認後，這份簡章與校系資料才會一起建立並發布。"}
                            </Dialog.Description>
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

                    {upload ? (
                        <div className="mt-6 space-y-5">
                            <div className="flex flex-wrap items-center justify-between gap-3 rounded-[var(--radius-small)] bg-ink/[0.04] px-4 py-3">
                                <div>
                                    <p className="max-w-2xl truncate font-sans text-sm font-bold text-ink">
                                        {upload.original_file_name}
                                    </p>
                                    <p className="mt-1 font-mono text-[11px] text-copy-muted">
                                        SHA {upload.sha256} · {formatBytes(upload.file_size_bytes)}
                                    </p>
                                </div>
                                <div className="flex flex-wrap items-center gap-2">
                                    <StatusBadge
                                        label={
                                            brochureUploadStatusLabel[
                                                effectiveStatus ?? upload.status
                                            ] ??
                                            effectiveStatus ??
                                            upload.status
                                        }
                                        status={effectiveStatus ?? upload.status}
                                    />
                                    <Button
                                        type="button"
                                        onClick={() => void download()}
                                        disabled={pendingAction !== null}
                                        className="h-10 gap-2 border border-ink/15 bg-surface px-4 text-sm text-ink hover:bg-ink/5"
                                    >
                                        {pendingAction === "download" ? (
                                            <Loader2 aria-hidden className="h-4 w-4 animate-spin" />
                                        ) : (
                                            <FileDown aria-hidden className="h-4 w-4" />
                                        )}
                                        開啟原始 PDF
                                    </Button>
                                </div>
                            </div>

                            <div className="grid items-start gap-5 lg:grid-cols-[minmax(0,0.95fr)_minmax(0,1.05fr)]">
                                <section className="overflow-hidden rounded-[var(--radius-small)] border border-ink/10 bg-ink/[0.03] lg:sticky lg:top-0">
                                    <div className="flex flex-wrap items-center justify-between gap-2 border-b border-ink/10 px-4 py-3">
                                        <div>
                                            <h3 className="font-serif text-lg text-ink">
                                                原始簡章 PDF
                                            </h3>
                                            <p className="mt-1 font-sans text-xs text-copy-muted">
                                                點選右側校系後，預覽會跳到該筆來源頁。
                                            </p>
                                        </div>
                                        <span className="rounded-full bg-ink/10 px-3 py-1 font-sans text-xs text-copy-muted">
                                            第 {selectedPdfPage ?? "—"} 頁
                                        </span>
                                    </div>
                                    {pdfPreviewSource ? (
                                        <iframe
                                            key={pdfPreviewSource}
                                            title={`${upload.original_file_name} PDF 預覽`}
                                            src={pdfPreviewSource}
                                            className="h-[34rem] w-full bg-white lg:h-[calc(100vh-16rem)] lg:min-h-[36rem]"
                                        />
                                    ) : pdfLoading ? (
                                        <div className="flex h-80 items-center justify-center">
                                            <LoadingState />
                                        </div>
                                    ) : (
                                        <p className="p-5 font-sans text-sm text-red-600">
                                            PDF 預覽載入失敗，請使用上方「開啟原始 PDF」查看。
                                        </p>
                                    )}
                                </section>

                                <div className="min-w-0 space-y-5">
                                    {detail === null ? (
                                        error ? (
                                            <p className="font-sans text-sm text-red-600">
                                                {error}
                                            </p>
                                        ) : (
                                            <LoadingState />
                                        )
                                    ) : (
                                        <>
                                            <div className="grid gap-4 rounded-[var(--radius-small)] border border-ink/10 p-4 sm:grid-cols-3">
                                                <DetailItem
                                                    label="系統辨識學年度"
                                                    value={
                                                        detail.detected_academic_year
                                                            ? String(detail.detected_academic_year)
                                                            : "待確認"
                                                    }
                                                />
                                                <DetailItem
                                                    label="系統辨識學校"
                                                    value={`${detectedText(detail.detected_school_code) || "待確認"} ／ ${detectedText(detail.detected_school_name) || "待確認"}`}
                                                />
                                                <DetailItem
                                                    label="候選校系數"
                                                    value={String(detail.candidates?.length ?? 0)}
                                                />
                                            </div>

                                            <div className="grid gap-4 sm:grid-cols-2">
                                                <Field label="最終學年度（人工確認）">
                                                    <input
                                                        className={`${inputClass} w-full`}
                                                        inputMode="numeric"
                                                        maxLength={3}
                                                        value={academicYear}
                                                        onChange={(event) =>
                                                            setAcademicYear(
                                                                event.target.value.replace(
                                                                    /\D/g,
                                                                    ""
                                                                )
                                                            )
                                                        }
                                                    />
                                                </Field>
                                                <Field label="最終學校編號（人工確認）">
                                                    <input
                                                        className={`${inputClass} w-full`}
                                                        inputMode="numeric"
                                                        maxLength={3}
                                                        value={schoolCode}
                                                        onChange={(event) =>
                                                            setSchoolCode(
                                                                event.target.value.replace(
                                                                    /\D/g,
                                                                    ""
                                                                )
                                                            )
                                                        }
                                                    />
                                                </Field>
                                            </div>

                                            <section className="rounded-[var(--radius-small)] border border-ink/10 p-4 sm:p-5">
                                                <div className="flex flex-wrap items-start justify-between gap-3">
                                                    <div>
                                                        <h3 className="font-serif text-lg text-ink">
                                                            建檔資料
                                                        </h3>
                                                        <p className="mt-1 font-sans text-sm leading-6 text-copy-muted">
                                                            以下欄位就是建立招生資料會使用的內容；學年度與學校編號由上方統一套用到所有校系。
                                                        </p>
                                                    </div>
                                                    <span className="rounded-full bg-ink/10 px-3 py-1 font-sans text-xs text-copy-muted">
                                                        {programs.length} 筆校系
                                                    </span>
                                                </div>

                                                {programs.length > 0 ? (
                                                    <>
                                                        <div className="mt-4 flex flex-wrap gap-2 border-b border-ink/10 pb-4">
                                                            {programs.map((program, index) => (
                                                                <button
                                                                    key={`brochure-program-${index}`}
                                                                    type="button"
                                                                    onClick={() =>
                                                                        setSelectedProgramIndex(
                                                                            index
                                                                        )
                                                                    }
                                                                    className={twMerge(
                                                                        "rounded-[var(--radius-small)] border px-3 py-2 text-left font-sans text-xs transition-colors",
                                                                        selectedProgramIndex ===
                                                                            index
                                                                            ? "border-ink bg-ink text-surface"
                                                                            : "border-ink/15 bg-surface text-ink hover:border-ink/40"
                                                                    )}
                                                                >
                                                                    <span className="block font-bold">
                                                                        {program.program_code ||
                                                                            "未填系所編號"}
                                                                    </span>
                                                                    <span className="mt-0.5 block max-w-44 truncate text-current/70">
                                                                        {program.admission_program_name ||
                                                                            `第 ${index + 1} 筆`}
                                                                    </span>
                                                                </button>
                                                            ))}
                                                        </div>
                                                        {programs[selectedProgramIndex] ? (
                                                            <BrochureProgramFields
                                                                program={
                                                                    programs[selectedProgramIndex]
                                                                }
                                                                onChange={(updated) =>
                                                                    updateProgram(
                                                                        selectedProgramIndex,
                                                                        updated
                                                                    )
                                                                }
                                                                onRemove={
                                                                    programs.length > 1
                                                                        ? () =>
                                                                              removeProgram(
                                                                                  selectedProgramIndex
                                                                              )
                                                                        : undefined
                                                                }
                                                            />
                                                        ) : null}
                                                    </>
                                                ) : (
                                                    <p className="mt-4 rounded-[var(--radius-small)] bg-accent-yellow/20 px-4 py-3 font-sans text-sm leading-6 text-ink">
                                                        系統尚未辨識到校系資料，請按下方按鈕手動新增一筆，再依
                                                        PDF 填寫。
                                                    </p>
                                                )}

                                                <Button
                                                    type="button"
                                                    onClick={addProgram}
                                                    className="mt-4 h-10 border border-ink/15 bg-surface px-4 text-sm text-ink hover:bg-ink/5"
                                                >
                                                    新增校系資料
                                                </Button>
                                            </section>

                                            {detail.candidates && detail.candidates.length > 0 ? (
                                                <section className="border-t border-ink/10 pt-5">
                                                    <h3 className="font-serif text-lg text-ink">
                                                        擷取證據
                                                    </h3>
                                                    <div className="mt-3 grid gap-3 sm:grid-cols-2">
                                                        {detail.candidates.map(
                                                            (candidate, index) => (
                                                                <div
                                                                    key={`evidence-${candidate.program_code}-${candidate.source_page ?? 0}-${index}`}
                                                                    className="rounded-[var(--radius-small)] border border-ink/10 p-3"
                                                                >
                                                                    <div className="flex flex-wrap items-center justify-between gap-2 font-sans text-xs">
                                                                        <span className="font-bold text-ink">
                                                                            校系{" "}
                                                                            {candidate.program_code}
                                                                        </span>
                                                                        <span className="text-copy-muted">
                                                                            第{" "}
                                                                            {candidate.source_page ||
                                                                                "—"}{" "}
                                                                            頁 · 信心度{" "}
                                                                            {formatConfidence(
                                                                                candidate.confidence
                                                                            )}
                                                                        </span>
                                                                    </div>
                                                                    <pre className="mt-2 max-h-40 overflow-auto font-mono text-[11px] leading-5 break-words whitespace-pre-wrap text-copy-muted">
                                                                        {JSON.stringify(
                                                                            candidate.data,
                                                                            null,
                                                                            2
                                                                        )}
                                                                    </pre>
                                                                </div>
                                                            )
                                                        )}
                                                    </div>
                                                </section>
                                            ) : effectiveStatus === "queued" ||
                                              effectiveStatus === "processing" ? (
                                                <p className="font-sans text-sm text-copy-muted">
                                                    簡章仍在解析中，辨識完成後會列出校系候選。
                                                </p>
                                            ) : (
                                                <p className="font-sans text-sm text-red-600">
                                                    目前沒有辨識到校系候選，請從原始 PDF
                                                    手動建立校系資料後再確認。
                                                </p>
                                            )}

                                            <ActionReason
                                                value={reason}
                                                onChange={setReason}
                                                label="複核備註／退回原因"
                                            />
                                            {error ? (
                                                <p className="font-sans text-sm text-red-600">
                                                    {error}
                                                </p>
                                            ) : null}
                                            <div className="flex flex-wrap justify-end gap-2">
                                                <Button
                                                    type="button"
                                                    onClick={() => void reject()}
                                                    disabled={
                                                        pendingAction !== null ||
                                                        effectiveStatus !== "pending_review"
                                                    }
                                                    className="h-10 justify-center bg-red-600 px-4 text-center text-sm hover:bg-red-700 active:bg-red-800"
                                                >
                                                    {pendingAction === "reject"
                                                        ? "退回中…"
                                                        : "退回簡章"}
                                                </Button>
                                                <Button
                                                    type="button"
                                                    onClick={() => void confirm()}
                                                    disabled={
                                                        pendingAction !== null ||
                                                        effectiveStatus !== "pending_review"
                                                    }
                                                    className="h-10 justify-center bg-accent-green-strong px-4 text-center text-sm text-ink hover:bg-accent-green"
                                                >
                                                    {pendingAction === "confirm"
                                                        ? "建立中…"
                                                        : isExternalSubmission
                                                          ? "確認並審核上架"
                                                          : "建立簡章"}
                                                </Button>
                                            </div>
                                        </>
                                    )}
                                </div>
                            </div>
                        </div>
                    ) : null}
                </Dialog.Content>
            </Dialog.Portal>
        </Dialog.Root>
    );
}

function BrochureProgramFields({
    program,
    onChange,
    onRemove
}: {
    program: AdmissionProgramInput;
    onChange: (program: AdmissionProgramInput) => void;
    onRemove?: () => void;
}) {
    function updateField<Key extends keyof AdmissionProgramInput>(
        key: Key,
        value: AdmissionProgramInput[Key]
    ) {
        onChange({ ...program, [key]: value });
    }

    function updateExamItem(index: number, patch: Partial<AdmissionExamItem>) {
        onChange({
            ...program,
            exam_items: program.exam_items.map((item, itemIndex) =>
                itemIndex === index ? { ...item, ...patch } : item
            )
        });
    }

    function addExamItem() {
        onChange({
            ...program,
            exam_items: [
                ...program.exam_items,
                {
                    name: "",
                    stage: "-",
                    sort_order: program.exam_items.length + 1,
                    description: "",
                    source_page: ""
                }
            ]
        });
    }

    function removeExamItem(index: number) {
        onChange({
            ...program,
            exam_items: program.exam_items
                .filter((_, itemIndex) => itemIndex !== index)
                .map((item, itemIndex) => ({ ...item, sort_order: itemIndex + 1 }))
        });
    }

    function updateTimelineEvent(index: number, patch: Partial<AdmissionTimelineEvent>) {
        onChange({
            ...program,
            timeline_events: program.timeline_events.map((event, eventIndex) =>
                eventIndex === index ? { ...event, ...patch } : event
            )
        });
    }

    function addTimelineEvent() {
        onChange({
            ...program,
            timeline_events: [
                ...program.timeline_events,
                {
                    name: "",
                    start_date: "-",
                    start_time: "-",
                    end_date: "-",
                    end_time: "-",
                    sort_order: program.timeline_events.length + 1,
                    notes: "-"
                }
            ]
        });
    }

    function removeTimelineEvent(index: number) {
        onChange({
            ...program,
            timeline_events: program.timeline_events
                .filter((_, eventIndex) => eventIndex !== index)
                .map((event, eventIndex) => ({ ...event, sort_order: eventIndex + 1 }))
        });
    }

    return (
        <div className="mt-5 flex flex-col gap-5">
            <div>
                <div className="flex flex-wrap items-center justify-between gap-2">
                    <h4 className="font-sans text-sm font-bold text-ink">校系基本資訊</h4>
                    {onRemove ? (
                        <button
                            type="button"
                            onClick={onRemove}
                            className="font-sans text-xs font-bold text-red-600 underline underline-offset-2"
                        >
                            移除這筆校系
                        </button>
                    ) : null}
                </div>
                <div className="mt-3 grid gap-4 sm:grid-cols-[8rem_minmax(0,1fr)_8rem]">
                    <Field label="系所編號">
                        <input
                            className={`${inputClass} w-full`}
                            inputMode="numeric"
                            maxLength={3}
                            placeholder="例如 023"
                            value={program.program_code}
                            onChange={(event) =>
                                updateField("program_code", event.target.value.replace(/\D/g, ""))
                            }
                        />
                    </Field>
                    <Field label="招生系所名稱">
                        <input
                            className={`${inputClass} w-full`}
                            placeholder="請依 PDF 填寫"
                            value={displayFormValue(program.admission_program_name)}
                            onChange={(event) =>
                                updateField("admission_program_name", event.target.value)
                            }
                        />
                    </Field>
                    <Field label="招生名額">
                        <input
                            className={`${inputClass} w-full`}
                            type="number"
                            min={0}
                            step={1}
                            value={program.admission_quota}
                            onChange={(event) =>
                                updateField(
                                    "admission_quota",
                                    Math.max(0, Number(event.target.value) || 0)
                                )
                            }
                        />
                    </Field>
                </div>
            </div>

            <div className="border-t border-ink/10 pt-5">
                <h4 className="font-sans text-sm font-bold text-ink">招生條件與備註</h4>
                <div className="mt-3 grid gap-4 sm:grid-cols-2">
                    <Field label="特殊才能對象">
                        <textarea
                            className={`${textareaClass} bg-surface font-sans text-sm`}
                            value={displayFormValue(program.special_talent_target)}
                            onChange={(event) =>
                                updateField("special_talent_target", event.target.value)
                            }
                        />
                    </Field>
                    <Field label="不同教育背景">
                        <textarea
                            className={`${textareaClass} bg-surface font-sans text-sm`}
                            value={displayFormValue(program.different_education_backgrounds)}
                            onChange={(event) =>
                                updateField("different_education_backgrounds", event.target.value)
                            }
                        />
                    </Field>
                    <Field label="其他招生條件補充">
                        <textarea
                            className={`${textareaClass} bg-surface font-sans text-sm`}
                            value={displayFormValue(program.different_education_other)}
                            onChange={(event) =>
                                updateField("different_education_other", event.target.value)
                            }
                        />
                    </Field>
                    <Field label="備註">
                        <textarea
                            className={`${textareaClass} bg-surface font-sans text-sm`}
                            value={displayFormValue(program.notes)}
                            onChange={(event) => updateField("notes", event.target.value)}
                        />
                    </Field>
                </div>
            </div>

            <div className="border-t border-ink/10 pt-5">
                <div className="flex flex-wrap items-start justify-between gap-3">
                    <div>
                        <h4 className="font-sans text-sm font-bold text-ink">招生時程</h4>
                        <p className="mt-1 font-sans text-xs leading-5 text-copy-muted">
                            依簡章公告順序新增每個時程項目（網路報名、二階名單公布、放榜、報到、遞補…）。只填開始日期／時間代表單一天或單一時間點；有結束日期才會顯示成時間段（例如報名期間 9/29～10/7）。
                        </p>
                    </div>
                    <Button
                        type="button"
                        onClick={addTimelineEvent}
                        className="h-9 border border-ink/15 bg-surface px-3 text-xs text-ink hover:bg-ink/5"
                    >
                        新增時程項目
                    </Button>
                </div>
                <div className="mt-3 flex flex-col gap-3">
                    {program.timeline_events.map((event, index) => (
                        <div
                            key={`timeline-event-${index}`}
                            className="rounded-[var(--radius-small)] border border-ink/10 bg-ink/[0.025] p-3"
                        >
                            <div className="flex items-center justify-between gap-2">
                                <span className="font-sans text-xs font-bold text-ink">
                                    時程項目 {index + 1}
                                </span>
                                <button
                                    type="button"
                                    onClick={() => removeTimelineEvent(index)}
                                    className="font-sans text-xs text-copy-muted underline underline-offset-2 hover:text-red-600"
                                >
                                    移除
                                </button>
                            </div>
                            <div className="mt-3 grid gap-3 sm:grid-cols-[minmax(0,1fr)_7rem]">
                                <Field label="項目名稱">
                                    <input
                                        className={`${inputClass} w-full`}
                                        placeholder="例如 網路報名、二階名單公布、正取生報到"
                                        value={displayFormValue(event.name)}
                                        onChange={(evt) =>
                                            updateTimelineEvent(index, { name: evt.target.value })
                                        }
                                    />
                                </Field>
                                <Field label="順序">
                                    <input
                                        className={`${inputClass} w-full`}
                                        type="number"
                                        min={1}
                                        step={1}
                                        value={event.sort_order}
                                        onChange={(evt) =>
                                            updateTimelineEvent(index, {
                                                sort_order: Math.max(
                                                    1,
                                                    Number(evt.target.value) || index + 1
                                                )
                                            })
                                        }
                                    />
                                </Field>
                            </div>
                            <div className="mt-3 grid gap-3 sm:grid-cols-2">
                                <DateField
                                    label="開始日期"
                                    value={event.start_date}
                                    onChange={(value) =>
                                        updateTimelineEvent(index, { start_date: value })
                                    }
                                />
                                <TimeField
                                    label="開始時間（24小時制）"
                                    value={event.start_time}
                                    onChange={(value) =>
                                        updateTimelineEvent(index, { start_time: value })
                                    }
                                />
                            </div>
                            <div className="mt-3 grid gap-3 sm:grid-cols-2">
                                <DateField
                                    label="結束日期（單一天/時間點不填）"
                                    value={event.end_date}
                                    onChange={(value) =>
                                        updateTimelineEvent(index, { end_date: value })
                                    }
                                />
                                <TimeField
                                    label="結束時間（24小時制）"
                                    value={event.end_time}
                                    onChange={(value) =>
                                        updateTimelineEvent(index, { end_time: value })
                                    }
                                />
                            </div>
                            <div className="mt-3">
                                <Field label="備註">
                                    <input
                                        className={`${inputClass} w-full`}
                                        value={displayFormValue(event.notes)}
                                        onChange={(evt) =>
                                            updateTimelineEvent(index, { notes: evt.target.value })
                                        }
                                    />
                                </Field>
                            </div>
                        </div>
                    ))}
                    {program.timeline_events.length === 0 ? (
                        <p className="rounded-[var(--radius-small)] bg-accent-yellow/20 px-3 py-2 font-sans text-sm text-ink">
                            尚未有時程項目（可留空，之後再補）。
                        </p>
                    ) : null}
                </div>
            </div>

            <div className="border-t border-ink/10 pt-5">
                <div className="flex flex-wrap items-start justify-between gap-3">
                    <div>
                        <h4 className="font-sans text-sm font-bold text-ink">考試項目</h4>
                        <p className="mt-1 font-sans text-xs leading-5 text-copy-muted">
                            每個項目至少要有名稱，並填寫比重或倍率；來源頁碼可用來回看 PDF。
                        </p>
                    </div>
                    <Button
                        type="button"
                        onClick={addExamItem}
                        className="h-9 border border-ink/15 bg-surface px-3 text-xs text-ink hover:bg-ink/5"
                    >
                        新增考試項目
                    </Button>
                </div>
                <div className="mt-3 flex flex-col gap-3">
                    {program.exam_items.map((item, index) => (
                        <div
                            key={`exam-item-${index}`}
                            className="rounded-[var(--radius-small)] border border-ink/10 bg-ink/[0.025] p-3"
                        >
                            <div className="flex items-center justify-between gap-2">
                                <span className="font-sans text-xs font-bold text-ink">
                                    考試項目 {index + 1}
                                </span>
                                <button
                                    type="button"
                                    onClick={() => removeExamItem(index)}
                                    className="font-sans text-xs text-copy-muted underline underline-offset-2 hover:text-red-600"
                                >
                                    移除
                                </button>
                            </div>
                            <div className="mt-3 grid gap-3 sm:grid-cols-[minmax(0,1fr)_7rem_7rem_6rem]">
                                <Field label="項目名稱">
                                    <input
                                        className={`${inputClass} w-full`}
                                        placeholder="例如 面試"
                                        value={displayFormValue(item.name)}
                                        onChange={(event) =>
                                            updateExamItem(index, { name: event.target.value })
                                        }
                                    />
                                </Field>
                                <Field label="順序">
                                    <input
                                        className={`${inputClass} w-full`}
                                        type="number"
                                        min={1}
                                        step={1}
                                        value={item.sort_order}
                                        onChange={(event) =>
                                            updateExamItem(index, {
                                                sort_order: Math.max(
                                                    1,
                                                    Number(event.target.value) || index + 1
                                                )
                                            })
                                        }
                                    />
                                </Field>
                                <Field label="比重 %">
                                    <input
                                        className={`${inputClass} w-full`}
                                        type="number"
                                        min={0}
                                        max={100}
                                        step="any"
                                        value={item.weight_percent ?? ""}
                                        onChange={(event) =>
                                            updateExamItem(index, {
                                                weight_percent: optionalNumber(event.target.value)
                                            })
                                        }
                                    />
                                </Field>
                                <Field label="倍率">
                                    <input
                                        className={`${inputClass} w-full`}
                                        type="number"
                                        min={0}
                                        step="any"
                                        value={item.multiplier ?? ""}
                                        onChange={(event) =>
                                            updateExamItem(index, {
                                                multiplier: optionalNumber(event.target.value)
                                            })
                                        }
                                    />
                                </Field>
                            </div>
                            <div className="mt-3 grid gap-3 sm:grid-cols-[7rem_minmax(0,1fr)_7rem]">
                                <Field label="階段">
                                    <input
                                        className={`${inputClass} w-full`}
                                        placeholder="初試／複試"
                                        value={displayFormValue(item.stage ?? "-")}
                                        onChange={(event) =>
                                            updateExamItem(index, { stage: event.target.value })
                                        }
                                    />
                                </Field>
                                <Field label="項目說明">
                                    <textarea
                                        className={`${textareaClass} min-h-20 bg-surface font-sans text-sm`}
                                        value={displayFormValue(item.description)}
                                        onChange={(event) =>
                                            updateExamItem(index, {
                                                description: event.target.value
                                            })
                                        }
                                    />
                                </Field>
                                <Field label="來源頁碼">
                                    <input
                                        className={`${inputClass} w-full`}
                                        inputMode="numeric"
                                        value={displayFormValue(item.source_page)}
                                        onChange={(event) =>
                                            updateExamItem(index, {
                                                source_page: event.target.value
                                            })
                                        }
                                    />
                                </Field>
                            </div>
                        </div>
                    ))}
                    {program.exam_items.length === 0 ? (
                        <p className="rounded-[var(--radius-small)] bg-accent-yellow/20 px-3 py-2 font-sans text-sm text-ink">
                            尚未有考試項目，請新增至少一項。
                        </p>
                    ) : null}
                </div>
            </div>

            <div className="border-t border-ink/10 pt-5">
                <h4 className="font-sans text-sm font-bold text-ink">報名費用與流程</h4>
                <div className="mt-3 grid gap-4 sm:grid-cols-2">
                    <Field label="報名費">
                        <input
                            className={`${inputClass} w-full`}
                            placeholder="例如 1000元"
                            value={displayFormValue(program.registration_fee)}
                            onChange={(event) =>
                                updateField("registration_fee", event.target.value)
                            }
                        />
                    </Field>
                    <Field label="甄試地點">
                        <input
                            className={`${inputClass} w-full`}
                            value={displayFormValue(program.exam_location)}
                            onChange={(event) => updateField("exam_location", event.target.value)}
                        />
                    </Field>
                    <DateField
                        label="推薦函截止日期（有值即代表需要推薦函）"
                        value={program.recommendation_letter_deadline}
                        onChange={(value) => updateField("recommendation_letter_deadline", value)}
                    />
                    <DateField
                        label="作品集截止日期（有值即代表需要作品集）"
                        value={program.portfolio_deadline}
                        onChange={(value) => updateField("portfolio_deadline", value)}
                    />
                    <Field label="報到與遞補流程">
                        <textarea
                            className={`${textareaClass} bg-surface font-sans text-sm`}
                            value={displayFormValue(program.checkin_waitlist_process)}
                            onChange={(event) =>
                                updateField("checkin_waitlist_process", event.target.value)
                            }
                        />
                    </Field>
                    <Field label="報名費減免資格">
                        <textarea
                            className={`${textareaClass} bg-surface font-sans text-sm`}
                            value={displayFormValue(program.fee_reduction_eligibility)}
                            onChange={(event) =>
                                updateField("fee_reduction_eligibility", event.target.value)
                            }
                        />
                    </Field>
                </div>
            </div>

            <div className="border-t border-ink/10 pt-5">
                <h4 className="font-sans text-sm font-bold text-ink">聯絡與來源</h4>
                <div className="mt-3 grid gap-4 sm:grid-cols-2">
                    <Field label="諮詢電話">
                        <input
                            className={`${inputClass} w-full`}
                            placeholder="含分機、聯絡人"
                            value={displayFormValue(program.consultation_phone)}
                            onChange={(event) =>
                                updateField("consultation_phone", event.target.value)
                            }
                        />
                    </Field>
                    <Field label="諮詢信箱">
                        <input
                            className={`${inputClass} w-full`}
                            type="email"
                            placeholder="沒有請留白"
                            value={displayFormValue(program.consultation_email ?? "-")}
                            onChange={(event) =>
                                updateField("consultation_email", event.target.value)
                            }
                        />
                    </Field>
                    <Field label="諮詢聯絡人／單位">
                        <input
                            className={`${inputClass} w-full`}
                            placeholder="人名或系所單位，沒有請留白"
                            value={displayFormValue(program.consultation_contact ?? "-")}
                            onChange={(event) =>
                                updateField("consultation_contact", event.target.value)
                            }
                        />
                    </Field>
                    <Field label="原始簡章網址">
                        <input
                            className={`${inputClass} w-full`}
                            type="url"
                            placeholder="沒有請留白"
                            value={displayFormValue(program.brochure_url)}
                            onChange={(event) => updateField("brochure_url", event.target.value)}
                        />
                    </Field>
                    <Field label="學校官網">
                        <input
                            className={`${inputClass} w-full`}
                            type="url"
                            placeholder="沒有請留白，須為 .edu.tw / .gov.tw 網域"
                            value={displayFormValue(program.school_official_url)}
                            onChange={(event) =>
                                updateField("school_official_url", event.target.value)
                            }
                        />
                    </Field>
                    <Field label="系網">
                        <input
                            className={`${inputClass} w-full`}
                            type="url"
                            placeholder="沒有請留白，須為 .edu.tw / .gov.tw 網域"
                            value={displayFormValue(program.department_official_url)}
                            onChange={(event) =>
                                updateField("department_official_url", event.target.value)
                            }
                        />
                    </Field>
                    <Field label="資料來源頁碼">
                        <input
                            className={`${inputClass} w-full`}
                            inputMode="numeric"
                            value={program.source_page ?? ""}
                            onChange={(event) =>
                                updateField(
                                    "source_page",
                                    event.target.value ? Number(event.target.value) : undefined
                                )
                            }
                        />
                    </Field>
                    <label className="flex items-center gap-2 self-end pb-2 font-sans text-sm text-ink">
                        <input
                            type="checkbox"
                            checked={program.brochure_is_tentative}
                            onChange={(event) =>
                                updateField("brochure_is_tentative", event.target.checked)
                            }
                        />
                        這是暫定簡章
                    </label>
                </div>
            </div>

            <div className="border-t border-ink/10 pt-5">
                <h4 className="font-sans text-sm font-bold text-ink">分類與統計</h4>
                <p className="mt-1 font-sans text-xs leading-5 text-copy-muted">
                    這些欄位個別學校簡章通常不會印出來；招生進行中可能還沒有數字，維持「-」即可。
                </p>
                <div className="mt-3 grid gap-4 sm:grid-cols-3">
                    <Field label="學群">
                        <input
                            className={`${inputClass} w-full`}
                            value={displayFormValue(program.admission_group)}
                            onChange={(event) =>
                                updateField("admission_group", event.target.value)
                            }
                        />
                    </Field>
                    <Field label="跨學群">
                        <input
                            className={`${inputClass} w-full`}
                            value={displayFormValue(program.cross_group)}
                            onChange={(event) => updateField("cross_group", event.target.value)}
                        />
                    </Field>
                    <Field label="學類">
                        <input
                            className={`${inputClass} w-full`}
                            value={displayFormValue(program.admission_category)}
                            onChange={(event) =>
                                updateField("admission_category", event.target.value)
                            }
                        />
                    </Field>
                    <Field label="是否優先錄取">
                        <input
                            className={`${inputClass} w-full`}
                            value={displayFormValue(program.priority_admission)}
                            onChange={(event) =>
                                updateField("priority_admission", event.target.value)
                            }
                        />
                    </Field>
                    <Field label="是否需要作品集">
                        <input
                            className={`${inputClass} w-full`}
                            value={displayFormValue(program.portfolio_required)}
                            onChange={(event) =>
                                updateField("portfolio_required", event.target.value)
                            }
                        />
                    </Field>
                    <Field label="推薦函暗/明函">
                        <input
                            className={`${inputClass} w-full`}
                            value={displayFormValue(program.recommendation_letter_type)}
                            onChange={(event) =>
                                updateField("recommendation_letter_type", event.target.value)
                            }
                        />
                    </Field>
                    <Field label="可報名學系數量">
                        <input
                            className={`${inputClass} w-full`}
                            value={displayFormValue(program.max_applicable_programs)}
                            onChange={(event) =>
                                updateField("max_applicable_programs", event.target.value)
                            }
                        />
                    </Field>
                    <Field label="報名人數">
                        <input
                            className={`${inputClass} w-full`}
                            value={displayFormValue(program.applicant_count)}
                            onChange={(event) =>
                                updateField("applicant_count", event.target.value)
                            }
                        />
                    </Field>
                    <Field label="面試人數">
                        <input
                            className={`${inputClass} w-full`}
                            value={displayFormValue(program.interview_count)}
                            onChange={(event) =>
                                updateField("interview_count", event.target.value)
                            }
                        />
                    </Field>
                    <Field label="正取人數">
                        <input
                            className={`${inputClass} w-full`}
                            value={displayFormValue(program.admitted_count)}
                            onChange={(event) =>
                                updateField("admitted_count", event.target.value)
                            }
                        />
                    </Field>
                    <Field label="備取人數">
                        <input
                            className={`${inputClass} w-full`}
                            value={displayFormValue(program.waitlisted_count)}
                            onChange={(event) =>
                                updateField("waitlisted_count", event.target.value)
                            }
                        />
                    </Field>
                    <Field label="錄取率">
                        <input
                            className={`${inputClass} w-full`}
                            value={displayFormValue(program.admission_rate)}
                            onChange={(event) =>
                                updateField("admission_rate", event.target.value)
                            }
                        />
                    </Field>
                    <Field label="初試通過率">
                        <input
                            className={`${inputClass} w-full`}
                            value={displayFormValue(program.first_stage_pass_rate)}
                            onChange={(event) =>
                                updateField("first_stage_pass_rate", event.target.value)
                            }
                        />
                    </Field>
                    <Field label="競爭倍率">
                        <input
                            className={`${inputClass} w-full`}
                            value={displayFormValue(program.competition_ratio)}
                            onChange={(event) =>
                                updateField("competition_ratio", event.target.value)
                            }
                        />
                    </Field>
                </div>
            </div>
        </div>
    );
}

function ProgramDialog({
    program,
    mfaCode,
    onClose,
    onChanged
}: {
    program: AdminAdmissionProgram | null;
    mfaCode: string;
    onClose: () => void;
    onChanged: (program: AdminAdmissionProgram) => void;
}) {
    const [reason, setReason] = useState("");
    const [pendingAction, setPendingAction] = useState<"approve" | "reject" | "save" | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [notice, setNotice] = useState<string | null>(null);
    const [history, setHistory] = useState<ProgramAuditEvent[] | null>(null);
    // Opens straight into the edit form rather than the read-only view —
    // staff are almost always cross-checking against the original PDF while
    // fixing a record, so an extra click into "編輯資料" first just gets in
    // the way. "回到檢視" still switches back for a quick read-only look.
    const [editing, setEditing] = useState(true);
    const [editItem, setEditItem] = useState<AdmissionProgramInput | null>(() =>
        program ? programToInput(program) : null
    );

    useEffect(() => {
        if (!program) return;
        let ignore = false;
        void listAdminAdmissionProgramHistory(program.program_identifier, mfaCode || undefined)
            .then((response) => {
                if (!ignore) setHistory(response.data);
            })
            .catch(() => {
                if (!ignore) setHistory([]);
            });
        return () => {
            ignore = true;
        };
    }, [mfaCode, program]);

    async function runReview(approved: boolean) {
        if (!program) return;
        if (!reason.trim()) {
            setError(approved ? "請填寫核准備註。" : "請填寫退回原因。");
            return;
        }
        setPendingAction(approved ? "approve" : "reject");
        setError(null);
        setNotice(null);
        try {
            const response = await reviewAdminAdmissionProgram(
                program.program_identifier,
                approved,
                reason.trim(),
                mfaCode || undefined
            );
            onChanged(response.data);
            setNotice(approved ? "招生資料已審核並上架。" : "招生資料已退回。 ");
            setReason("");
            const historyResponse = await listAdminAdmissionProgramHistory(
                program.program_identifier,
                mfaCode || undefined
            );
            setHistory(historyResponse.data);
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setPendingAction(null);
        }
    }

    async function saveEdit() {
        if (!program || !editItem) return;
        if (!reason.trim()) {
            setError("請填寫此次修正原因。");
            return;
        }
        const item: AdmissionProgramInput = {
            ...editItem,
            academic_year: program.academic_year,
            school_code: program.school_code,
            program_code: program.program_code
        };
        setPendingAction("save");
        setError(null);
        setNotice(null);
        try {
            const response = await updateAdminAdmissionProgram(
                program.program_identifier,
                reason.trim(),
                item,
                mfaCode || undefined
            );
            onChanged(response.data);
            setNotice("招生資料已更新，狀態回到待審核。 ");
            setReason("");
            setEditing(false);
            setEditItem(programToInput(response.data));
            const historyResponse = await listAdminAdmissionProgramHistory(
                program.program_identifier,
                mfaCode || undefined
            );
            setHistory(historyResponse.data);
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setPendingAction(null);
        }
    }

    return (
        <Dialog.Root open={program !== null} onOpenChange={(open) => !open && onClose()}>
            <Dialog.Portal>
                <Dialog.Overlay className="fixed inset-0 z-[80] bg-ink/40" />
                <Dialog.Content className="fixed top-1/2 left-1/2 z-[90] max-h-[calc(100vh-2rem)] w-[calc(100vw-2rem)] max-w-7xl -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-[var(--radius-panel)] bg-surface p-6 shadow-[var(--shadow-card)] sm:p-8">
                    <div className="flex items-start justify-between gap-4">
                        <div>
                            <Dialog.Title className="font-serif text-2xl text-ink">
                                {program?.admission_program_name ?? "招生資料"}
                            </Dialog.Title>
                            {program ? (
                                <p className="mt-1 font-mono text-xs text-copy-muted">
                                    {program.program_identifier} · {program.school_name}
                                </p>
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

                    {program ? (
                        <div className="mt-6 flex flex-col gap-6">
                            <div className="flex flex-wrap items-center justify-between gap-3 rounded-[var(--radius-small)] bg-ink/[0.04] px-4 py-3">
                                <div className="flex items-center gap-3">
                                    <span className="font-sans text-sm text-copy-muted">
                                        目前狀態
                                    </span>
                                    <StatusBadge
                                        label={
                                            programStatusLabel[program.review_status] ??
                                            program.review_status
                                        }
                                        status={program.review_status}
                                    />
                                </div>
                                <button
                                    type="button"
                                    onClick={() => {
                                        setEditing((value) => !value);
                                        setError(null);
                                        setNotice(null);
                                    }}
                                    className="font-sans text-xs font-bold text-ink underline underline-offset-2"
                                >
                                    {editing ? "回到檢視" : "編輯資料"}
                                </button>
                            </div>

                            {editing && editItem ? (
                                <div className="grid items-start gap-5 lg:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
                                    <ProgramBrochurePreview program={program} />
                                    <div className="flex min-w-0 flex-col gap-3">
                                        <p className="font-sans text-sm leading-6 text-copy-muted">
                                            編輯內容會重新進入待審核。學年度、學校編號、科系編號在這裡不能改。
                                        </p>
                                        <BrochureProgramFields
                                            program={editItem}
                                            onChange={setEditItem}
                                        />
                                        <ActionReason
                                            value={reason}
                                            onChange={setReason}
                                            label="修正原因（必填）"
                                        />
                                        <div className="flex flex-wrap gap-2">
                                            <Button
                                                type="button"
                                                onClick={() => void saveEdit()}
                                                disabled={pendingAction !== null}
                                                className="h-10 gap-2 px-4 text-sm"
                                            >
                                                {pendingAction === "save" ? (
                                                    <Loader2
                                                        aria-hidden
                                                        className="h-4 w-4 animate-spin"
                                                    />
                                                ) : null}
                                                儲存修正
                                            </Button>
                                            <Button
                                                type="button"
                                                onClick={() => setEditItem(programToInput(program))}
                                                className="h-10 border border-ink/15 bg-surface px-4 text-sm text-ink hover:bg-ink/5"
                                            >
                                                還原目前資料
                                            </Button>
                                        </div>
                                    </div>
                                </div>
                            ) : (
                                <ProgramDetails program={program} />
                            )}

                            {!editing && program.review_status === "pending" ? (
                                <div className="border-t border-ink/10 pt-5">
                                    <ActionReason
                                        value={reason}
                                        onChange={setReason}
                                        label="審核備註（核准／退回皆建議填寫）"
                                    />
                                    <div className="mt-3 flex flex-wrap gap-2">
                                        <Button
                                            type="button"
                                            onClick={() => void runReview(true)}
                                            disabled={pendingAction !== null}
                                            className="h-10 gap-2 bg-accent-green-strong px-4 text-sm text-ink hover:bg-accent-green"
                                        >
                                            {pendingAction === "approve" ? (
                                                <Loader2
                                                    aria-hidden
                                                    className="h-4 w-4 animate-spin"
                                                />
                                            ) : (
                                                <Check aria-hidden className="h-4 w-4" />
                                            )}
                                            核准並上架
                                        </Button>
                                        <Button
                                            type="button"
                                            onClick={() => void runReview(false)}
                                            disabled={pendingAction !== null}
                                            className="h-10 bg-red-600 px-4 text-sm hover:bg-red-700 active:bg-red-800"
                                        >
                                            退回
                                        </Button>
                                    </div>
                                </div>
                            ) : null}

                            {error ? (
                                <p className="font-sans text-sm text-red-600">{error}</p>
                            ) : null}
                            {notice ? (
                                <p className="font-sans text-sm text-ink/75">{notice}</p>
                            ) : null}

                            <ProgramYearHistorySection program={program} mfaCode={mfaCode} />

                            <HistoryBlock history={history} />
                        </div>
                    ) : null}
                </Dialog.Content>
            </Dialog.Portal>
        </Dialog.Root>
    );
}

// Lets staff backfill past years' 報名/正取/候補 counts for 歷年招生資料.
function ProgramYearHistorySection({
    program,
    mfaCode
}: {
    program: AdminAdmissionProgram;
    mfaCode: string;
}) {
    const [years, setYears] = useState<AdminAdmissionProgram[] | null>(null);
    const [adding, setAdding] = useState(false);
    const [newYear, setNewYear] = useState("");
    const [applicantCount, setApplicantCount] = useState("");
    const [admittedCount, setAdmittedCount] = useState("");
    const [waitlistedCount, setWaitlistedCount] = useState("");
    const [pending, setPending] = useState(false);
    const [error, setError] = useState<string | null>(null);

    function reload() {
        return listAdminAdmissionPrograms(
            { schoolCode: program.school_code, programCode: program.program_code, limit: 100 },
            mfaCode || undefined
        )
            .then((response) => {
                setYears([...response.data].sort((a, b) => b.academic_year - a.academic_year));
            })
            .catch(() => setYears([]));
    }

    useEffect(() => {
        void reload();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [program.school_code, program.program_code, mfaCode]);

    async function submitNewYear() {
        setError(null);
        const year = Number(newYear);
        if (!Number.isInteger(year) || year < 100 || year > 999) {
            setError("學年度必須是 3 碼數字（例如 114）。");
            return;
        }
        if (years?.some((entry) => entry.academic_year === year)) {
            setError(`${year} 學年度已經有資料了，請直接在下方列表修改。`);
            return;
        }
        setPending(true);
        try {
            const item: AdmissionProgramInput = {
                ...programToInput(program),
                academic_year: year,
                applicant_count: applicantCount.trim() || "-",
                admitted_count: admittedCount.trim() || "-",
                waitlisted_count: waitlistedCount.trim() || "-"
            };
            const syncResponse = await syncAdminAdmissionPrograms(
                `補登 ${year} 學年度歷史招生資料`,
                [item],
                mfaCode || undefined
            );
            const created = syncResponse.data[0];
            if (created && created.review_status !== "published") {
                await reviewAdminAdmissionProgram(
                    created.program_identifier,
                    true,
                    "歷史資料補登，直接上架",
                    mfaCode || undefined
                );
            }
            setNewYear("");
            setApplicantCount("");
            setAdmittedCount("");
            setWaitlistedCount("");
            setAdding(false);
            await reload();
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setPending(false);
        }
    }

    return (
        <div className="border-t border-ink/10 pt-5">
            <div className="flex items-center justify-between gap-3">
                <h4 className="font-sans text-sm font-bold text-ink">歷年招生資料</h4>
                <button
                    type="button"
                    onClick={() => setAdding((value) => !value)}
                    className="font-sans text-xs font-bold text-ink underline underline-offset-2"
                >
                    {adding ? "取消新增" : "+ 新增歷史年度"}
                </button>
            </div>
            <p className="mt-1 font-sans text-xs leading-5 text-copy-muted">
                依「學校代碼＋科系代碼」歸類，同一個科系每個學年度各一筆。今年把舊年度資料補齊後，之後每年資料同步時會自動歸類進來，不用另外維護。
            </p>

            {adding ? (
                <div className="mt-3 flex flex-wrap items-end gap-3 rounded-[var(--radius-small)] bg-ink/[0.03] p-3">
                    <Field label="學年度">
                        <input
                            className={`${inputClass} w-24`}
                            value={newYear}
                            onChange={(event) => setNewYear(event.target.value)}
                            placeholder="114"
                        />
                    </Field>
                    <Field label="報名人數">
                        <input
                            className={`${inputClass} w-24`}
                            value={applicantCount}
                            onChange={(event) => setApplicantCount(event.target.value)}
                        />
                    </Field>
                    <Field label="正取人數">
                        <input
                            className={`${inputClass} w-24`}
                            value={admittedCount}
                            onChange={(event) => setAdmittedCount(event.target.value)}
                        />
                    </Field>
                    <Field label="備取人數">
                        <input
                            className={`${inputClass} w-24`}
                            value={waitlistedCount}
                            onChange={(event) => setWaitlistedCount(event.target.value)}
                        />
                    </Field>
                    <Button
                        type="button"
                        onClick={() => void submitNewYear()}
                        disabled={pending}
                        className="h-10 gap-2 px-4 text-sm"
                    >
                        {pending ? <Loader2 aria-hidden className="h-4 w-4 animate-spin" /> : null}
                        {pending ? "建立中…" : "建立"}
                    </Button>
                </div>
            ) : null}
            {error ? <p className="mt-2 font-sans text-sm text-red-600">{error}</p> : null}

            {years === null ? (
                <p className="mt-3 font-sans text-sm text-copy-muted">載入中…</p>
            ) : years.length === 0 ? (
                <p className="mt-3 font-sans text-sm text-copy-muted">目前沒有任何學年度資料。</p>
            ) : (
                <div className="mt-3 overflow-x-auto">
                    <table className="w-full min-w-[28rem] border-collapse text-left font-sans text-sm">
                        <thead className="text-copy-muted">
                            <tr className="border-b border-ink/10">
                                <th className="py-1.5 pr-3 font-medium">學年度</th>
                                <th className="py-1.5 pr-3 font-medium">報名人數</th>
                                <th className="py-1.5 pr-3 font-medium">正取人數</th>
                                <th className="py-1.5 pr-3 font-medium">備取人數</th>
                                <th className="py-1.5 pr-3 font-medium">狀態</th>
                            </tr>
                        </thead>
                        <tbody className="divide-y divide-ink/10 text-ink/80">
                            {years.map((entry) => (
                                <tr key={entry.program_identifier}>
                                    <td className="py-1.5 pr-3">
                                        {entry.academic_year}
                                        {entry.academic_year === program.academic_year ? "（目前）" : ""}
                                    </td>
                                    <td className="py-1.5 pr-3">{displayFormValue(entry.applicant_count)}</td>
                                    <td className="py-1.5 pr-3">{displayFormValue(entry.admitted_count)}</td>
                                    <td className="py-1.5 pr-3">{displayFormValue(entry.waitlisted_count)}</td>
                                    <td className="py-1.5 pr-3">
                                        {programStatusLabel[entry.review_status] ?? entry.review_status}
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}
        </div>
    );
}

function BrochureDialog({
    brochure,
    mfaCode,
    onClose,
    onChanged
}: {
    brochure: BrochureDocument | null;
    mfaCode: string;
    onClose: () => void;
    onChanged: (brochure: BrochureDocument) => void;
}) {
    const [reason, setReason] = useState("");
    const [pendingAction, setPendingAction] = useState<
        "approve" | "reject" | "publish" | "unpublish" | "download" | null
    >(null);
    const [error, setError] = useState<string | null>(null);
    const [notice, setNotice] = useState<string | null>(null);
    const [events, setEvents] = useState<BrochureEvent[] | null>(null);

    useEffect(() => {
        if (!brochure) return;
        let ignore = false;
        void listAdminBrochureEvents(
            brochure.academic_year,
            brochure.school_code,
            mfaCode || undefined
        )
            .then((response) => {
                if (!ignore) setEvents(response.data);
            })
            .catch(() => {
                if (!ignore) setEvents([]);
            });
        return () => {
            ignore = true;
        };
    }, [brochure, mfaCode]);

    async function run(action: "approve" | "reject" | "publish" | "unpublish") {
        if (!brochure) return;
        if (!reason.trim()) {
            setError(
                action === "reject" || action === "unpublish" ? "請填寫原因。" : "請填寫審核備註。"
            );
            return;
        }
        setPendingAction(action);
        setError(null);
        setNotice(null);
        try {
            let updated: BrochureDocument;
            if (action === "approve" || action === "reject") {
                updated = (
                    await reviewAdminBrochure(
                        brochure.academic_year,
                        brochure.school_code,
                        action === "approve",
                        reason.trim(),
                        mfaCode || undefined
                    )
                ).data;
            } else {
                updated = (
                    await setAdminBrochureVisibility(
                        brochure.academic_year,
                        brochure.school_code,
                        action === "publish",
                        reason.trim(),
                        mfaCode || undefined
                    )
                ).data;
            }
            onChanged(updated);
            setReason("");
            setNotice(
                action === "approve"
                    ? "簡章已審核並上架。"
                    : action === "reject"
                      ? "簡章已退回。"
                      : action === "publish"
                        ? "簡章已重新上架。"
                        : "簡章已下架。"
            );
            const eventsResponse = await listAdminBrochureEvents(
                brochure.academic_year,
                brochure.school_code,
                mfaCode || undefined
            );
            setEvents(eventsResponse.data);
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setPendingAction(null);
        }
    }

    async function download() {
        if (!brochure) return;
        setPendingAction("download");
        setError(null);
        try {
            const response = await getAdminBrochureDownload(
                brochure.academic_year,
                brochure.school_code,
                mfaCode || undefined
            );
            window.open(response.url, "_blank", "noopener,noreferrer");
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setPendingAction(null);
        }
    }

    return (
        <Dialog.Root open={brochure !== null} onOpenChange={(open) => !open && onClose()}>
            <Dialog.Portal>
                <Dialog.Overlay className="fixed inset-0 z-[80] bg-ink/40" />
                <Dialog.Content className="fixed top-1/2 left-1/2 z-[90] max-h-[calc(100vh-2rem)] w-[calc(100vw-2rem)] max-w-2xl -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-[var(--radius-panel)] bg-surface p-6 shadow-[var(--shadow-card)] sm:p-8">
                    <div className="flex items-start justify-between gap-4">
                        <div>
                            <Dialog.Title className="font-serif text-2xl text-ink">
                                簡章檔案管理
                            </Dialog.Title>
                            {brochure ? (
                                <p className="mt-1 font-mono text-xs text-copy-muted">
                                    {brochure.academic_year} ／ {brochure.school_code}
                                </p>
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

                    {brochure ? (
                        <div className="mt-6 flex flex-col gap-5">
                            <div className="flex flex-wrap items-center justify-between gap-3 rounded-[var(--radius-small)] bg-ink/[0.04] px-4 py-3">
                                <StatusBadge
                                    label={
                                        brochureStatusLabel[brochure.review_status] ??
                                        brochure.review_status
                                    }
                                    status={brochure.review_status}
                                />
                                <Button
                                    type="button"
                                    onClick={() => void download()}
                                    disabled={pendingAction !== null}
                                    className="h-10 gap-2 border border-ink/15 bg-surface px-4 text-sm text-ink hover:bg-ink/5"
                                >
                                    {pendingAction === "download" ? (
                                        <Loader2 aria-hidden className="h-4 w-4 animate-spin" />
                                    ) : (
                                        <FileDown aria-hidden className="h-4 w-4" />
                                    )}
                                    下載檔案
                                </Button>
                            </div>

                            <dl className="grid gap-x-6 gap-y-4 font-sans text-sm sm:grid-cols-2">
                                <DetailItem label="檔名" value={brochure.original_file_name} />
                                <DetailItem
                                    label="檔案大小"
                                    value={formatBytes(brochure.file_size_bytes)}
                                />
                                <DetailItem label="MIME Type" value={brochure.mime_type} />
                                <DetailItem label="SHA-256" value={brochure.sha256} mono />
                                <DetailItem
                                    label="建立時間"
                                    value={formatDate(brochure.created_at)}
                                />
                                <DetailItem
                                    label="更新時間"
                                    value={formatDate(brochure.updated_at)}
                                />
                            </dl>

                            {brochure.source_url ? (
                                <a
                                    href={brochure.source_url}
                                    target="_blank"
                                    rel="noreferrer"
                                    className="flex items-center gap-2 font-sans text-sm font-bold text-ink underline underline-offset-2"
                                >
                                    <ExternalLink aria-hidden className="h-4 w-4" />
                                    開啟官方來源網址
                                </a>
                            ) : null}

                            {brochure.review_status === "pending" ? (
                                <div className="border-t border-ink/10 pt-5">
                                    <ActionReason
                                        value={reason}
                                        onChange={setReason}
                                        label="審核備註（必填）"
                                    />
                                    <div className="mt-3 flex flex-wrap gap-2">
                                        <Button
                                            type="button"
                                            onClick={() => void run("approve")}
                                            disabled={pendingAction !== null}
                                            className="h-10 gap-2 bg-accent-green-strong px-4 text-sm text-ink hover:bg-accent-green"
                                        >
                                            {pendingAction === "approve" ? (
                                                <Loader2
                                                    aria-hidden
                                                    className="h-4 w-4 animate-spin"
                                                />
                                            ) : (
                                                <Check aria-hidden className="h-4 w-4" />
                                            )}
                                            核准並上架
                                        </Button>
                                        <Button
                                            type="button"
                                            onClick={() => void run("reject")}
                                            disabled={pendingAction !== null}
                                            className="h-10 bg-red-600 px-4 text-sm hover:bg-red-700 active:bg-red-800"
                                        >
                                            退回
                                        </Button>
                                    </div>
                                </div>
                            ) : null}

                            {brochure.review_status === "published" ? (
                                <div className="border-t border-ink/10 pt-5">
                                    <ActionReason
                                        value={reason}
                                        onChange={setReason}
                                        label="下架原因（必填）"
                                    />
                                    <Button
                                        type="button"
                                        onClick={() => void run("unpublish")}
                                        disabled={pendingAction !== null}
                                        className="mt-3 h-10 gap-2 bg-red-600 px-4 text-sm hover:bg-red-700 active:bg-red-800"
                                    >
                                        <EyeOff aria-hidden className="h-4 w-4" />
                                        下架
                                    </Button>
                                </div>
                            ) : null}

                            {brochure.review_status === "archived" ? (
                                <div className="border-t border-ink/10 pt-5">
                                    <ActionReason
                                        value={reason}
                                        onChange={setReason}
                                        label="重新上架備註（必填）"
                                    />
                                    <Button
                                        type="button"
                                        onClick={() => void run("publish")}
                                        disabled={pendingAction !== null}
                                        className="mt-3 h-10 gap-2 bg-accent-green-strong px-4 text-sm text-ink hover:bg-accent-green"
                                    >
                                        <Eye aria-hidden className="h-4 w-4" />
                                        重新上架
                                    </Button>
                                </div>
                            ) : null}

                            {error ? (
                                <p className="font-sans text-sm text-red-600">{error}</p>
                            ) : null}
                            {notice ? (
                                <p className="font-sans text-sm text-ink/75">{notice}</p>
                            ) : null}

                            <BrochureHistory events={events} />
                        </div>
                    ) : null}
                </Dialog.Content>
            </Dialog.Portal>
        </Dialog.Root>
    );
}

function SyncDialog({
    open,
    mfaCode,
    onClose,
    onSynced
}: {
    open: boolean;
    mfaCode: string;
    onClose: () => void;
    onSynced: () => void;
}) {
    const [reason, setReason] = useState("教育部官方招生資料同步");
    const [payload, setPayload] = useState(() => JSON.stringify([syncTemplate], null, 2));
    const [pending, setPending] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [notice, setNotice] = useState<string | null>(null);

    async function submit(event: React.FormEvent) {
        event.preventDefault();
        setError(null);
        setNotice(null);
        if (!reason.trim()) {
            setError("請填寫同步原因。");
            return;
        }
        let items: AdmissionProgramInput[];
        try {
            const parsed: unknown = JSON.parse(payload);
            if (!Array.isArray(parsed) || parsed.length === 0) throw new Error("invalid_items");
            items = parsed as AdmissionProgramInput[];
        } catch {
            setError("同步資料必須是至少包含一筆資料的 JSON 陣列。");
            return;
        }
        setPending(true);
        try {
            const response = await syncAdminAdmissionPrograms(
                reason.trim(),
                items,
                mfaCode || undefined
            );
            setNotice(
                `已同步 ${response.meta?.count ?? response.data.length} 筆資料；變更項目會進入待審核。`
            );
            onSynced();
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setPending(false);
        }
    }

    return (
        <Dialog.Root open={open} onOpenChange={(value) => !value && onClose()}>
            <Dialog.Portal>
                <Dialog.Overlay className="fixed inset-0 z-[80] bg-ink/40" />
                <Dialog.Content className="fixed top-1/2 left-1/2 z-[90] max-h-[calc(100vh-2rem)] w-[calc(100vw-2rem)] max-w-3xl -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-[var(--radius-panel)] bg-surface p-6 shadow-[var(--shadow-card)] sm:p-8">
                    <div className="flex items-start justify-between gap-4">
                        <div>
                            <Dialog.Title className="font-serif text-2xl text-ink">
                                批次同步招生資料
                            </Dialog.Title>
                            <Dialog.Description className="mt-2 max-w-2xl font-sans text-sm leading-6 text-copy-muted">
                                貼上符合 Admission Program API 的 JSON 陣列。每批最多 500
                                筆；學校名稱由後端主檔解析，識別碼與來源定位不接受手動覆蓋。
                            </Dialog.Description>
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

                    <form onSubmit={submit} className="mt-6 flex flex-col gap-4">
                        <Field label="同步原因（必填）">
                            <input
                                className={`${inputClass} w-full`}
                                maxLength={2000}
                                value={reason}
                                onChange={(event) => setReason(event.target.value)}
                            />
                        </Field>
                        <Field label="招生資料 JSON 陣列">
                            <textarea
                                className={`${textareaClass} min-h-[30rem]`}
                                value={payload}
                                onChange={(event) => setPayload(event.target.value)}
                                spellCheck={false}
                            />
                        </Field>
                        {error ? <p className="font-sans text-sm text-red-600">{error}</p> : null}
                        {notice ? <p className="font-sans text-sm text-ink/75">{notice}</p> : null}
                        <div className="flex flex-wrap gap-2">
                            <Button
                                type="submit"
                                disabled={pending}
                                className="h-10 gap-2 px-4 text-sm"
                            >
                                {pending ? (
                                    <Loader2 aria-hidden className="h-4 w-4 animate-spin" />
                                ) : null}
                                {pending ? "同步中…" : "送出同步"}
                            </Button>
                            <Button
                                type="button"
                                onClick={() => setPayload(JSON.stringify([syncTemplate], null, 2))}
                                className="h-10 border border-ink/15 bg-surface px-4 text-sm text-ink hover:bg-ink/5"
                            >
                                套用範例格式
                            </Button>
                        </div>
                    </form>
                </Dialog.Content>
            </Dialog.Portal>
        </Dialog.Root>
    );
}

function ProgramDetails({ program }: { program: AdminAdmissionProgram }) {
    return (
        <div className="grid items-start gap-5 lg:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
            <ProgramBrochurePreview program={program} />
            <div className="flex min-w-0 flex-col gap-5">
            <div className="grid gap-x-6 gap-y-4 font-sans text-sm sm:grid-cols-3">
                <DetailItem label="學年度" value={String(program.academic_year)} />
                <DetailItem
                    label="校系編號"
                    value={`${program.school_code} ／ ${program.program_code}`}
                    mono
                />
                <DetailItem label="招生名額" value={String(program.admission_quota)} />
                <DetailItem
                    label="簡章狀態"
                    value={program.brochure_is_tentative ? "暫定" : "正式"}
                />
                <DetailItem label="諮詢電話" value={program.consultation_phone} />
                <DetailItem label="諮詢信箱" value={program.consultation_email} />
                <DetailItem label="諮詢聯絡人／單位" value={program.consultation_contact} />
                <DetailItem label="來源定位" value={program.source_locator} mono />
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
                <TextBlock title="特殊才能對象" value={program.special_talent_target} />
                <TextBlock title="不同教育背景" value={program.different_education_backgrounds} />
            </div>
            <TextBlock title="考試項目" value="">
                <div className="overflow-x-auto rounded-[var(--radius-small)] border border-ink/10">
                    <table className="w-full min-w-[520px] font-sans text-sm">
                        <thead>
                            <tr className="border-b border-ink/10 text-left text-copy-muted">
                                <Th>順序</Th>
                                <Th>階段</Th>
                                <Th>項目</Th>
                                <Th>比重／倍率</Th>
                                <Th>來源頁</Th>
                            </tr>
                        </thead>
                        <tbody>
                            {program.exam_items.map((item) => (
                                <tr
                                    key={`${item.sort_order}-${item.name}`}
                                    className="border-b border-ink/5 last:border-0"
                                >
                                    <td className="px-3 py-2">{item.sort_order}</td>
                                    <td className="px-3 py-2 text-copy-muted">{item.stage}</td>
                                    <td className="px-3 py-2">
                                        <p className="font-bold text-ink">{item.name}</p>
                                        <p className="mt-1 text-xs text-copy-muted">
                                            {item.description}
                                        </p>
                                    </td>
                                    <td className="px-3 py-2 text-copy-muted">
                                        {formatExamWeight(item)}
                                    </td>
                                    <td className="px-3 py-2 text-copy-muted">
                                        {item.source_page}
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            </TextBlock>
            <div className="grid gap-4 sm:grid-cols-2">
                <TextBlock title="招生條件補充" value={program.different_education_other} />
                <TextBlock title="備註" value={program.notes} />
            </div>
            </div>
        </div>
    );
}

// Streams the PDF through the API (not a signed storage URL) so it works
// even when the browser can't resolve the storage host directly.
function ProgramBrochurePreview({ program }: { program: AdminAdmissionProgram }) {
    const [url, setUrl] = useState<string | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        let ignore = false;
        let objectURL: string | null = null;
        setLoading(true);
        setError(null);
        setUrl(null);

        void fetch(getAdminBrochureContentURL(program.academic_year, program.school_code), {
            credentials: "include",
            headers: { Accept: "application/pdf" }
        })
            .then(async (response) => {
                if (!response.ok) {
                    let message = "PDF 預覽載入失敗。";
                    try {
                        const body = (await response.json()) as {
                            error?: { code?: string; message?: string };
                        };
                        message = body.error?.message || message;
                    } catch {
                        // Keep the friendly fallback when the error is not JSON.
                    }
                    throw new ApiError(response.status, "pdf_preview_failed", message);
                }
                const blob = await response.blob();
                objectURL = URL.createObjectURL(blob);
                if (!ignore) {
                    setUrl(objectURL);
                } else {
                    URL.revokeObjectURL(objectURL);
                }
            })
            .catch((cause) => {
                if (!ignore) setError(describeError(cause));
            })
            .finally(() => {
                if (!ignore) setLoading(false);
            });

        return () => {
            ignore = true;
            if (objectURL) URL.revokeObjectURL(objectURL);
        };
    }, [program.academic_year, program.school_code]);

    const pageMatch = program.source_locator.match(/-(\d{1,3})$/);
    const page = pageMatch ? Number(pageMatch[1]) : undefined;
    const previewSource = url
        ? `${url}#${page ? `page=${page}&` : ""}navpanes=0&zoom=page-fit`
        : null;

    return (
        <section className="overflow-hidden rounded-[var(--radius-small)] border border-ink/10 bg-ink/[0.03] lg:sticky lg:top-0">
            <div className="flex flex-wrap items-center justify-between gap-2 border-b border-ink/10 px-4 py-3">
                <div>
                    <h3 className="font-serif text-lg text-ink">原始簡章 PDF</h3>
                    <p className="mt-1 font-sans text-xs text-copy-muted">
                        預設跳到本校系的來源頁。
                    </p>
                </div>
                <span className="rounded-full bg-ink/10 px-3 py-1 font-sans text-xs text-copy-muted">
                    第 {page ?? "—"} 頁
                </span>
            </div>
            {previewSource ? (
                <iframe
                    key={previewSource}
                    title={`${program.admission_program_name} 原始簡章 PDF 預覽`}
                    src={previewSource}
                    className="h-[28rem] w-full bg-white lg:h-[calc(100vh-20rem)] lg:min-h-[28rem]"
                />
            ) : loading ? (
                <div className="flex h-72 items-center justify-center">
                    <LoadingState />
                </div>
            ) : (
                <p className="p-5 font-sans text-sm text-red-600">
                    {error || "PDF 預覽載入失敗。"}
                </p>
            )}
        </section>
    );
}

function HistoryBlock({ history }: { history: ProgramAuditEvent[] | null }) {
    return (
        <section className="border-t border-ink/10 pt-5">
            <div className="flex items-center gap-2">
                <History aria-hidden className="h-4 w-4 text-ink/60" />
                <h3 className="font-serif text-lg text-ink">資料異動紀錄</h3>
            </div>
            {history === null ? (
                <p className="mt-3 font-sans text-sm text-copy-muted">載入中…</p>
            ) : history.length === 0 ? (
                <p className="mt-3 font-sans text-sm text-copy-muted">尚無異動紀錄。</p>
            ) : (
                <div className="mt-3 flex flex-col gap-2">
                    {history.map((event) => (
                        <div
                            key={event.id}
                            className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 rounded-[var(--radius-small)] bg-ink/[0.03] px-3 py-2 font-sans text-xs"
                        >
                            <span className="font-bold text-ink">{event.action}</span>
                            <span className="text-copy-muted">{event.reason || "—"}</span>
                            <time className="text-copy-muted">{formatDate(event.created_at)}</time>
                        </div>
                    ))}
                </div>
            )}
        </section>
    );
}

function BrochureHistory({ events }: { events: BrochureEvent[] | null }) {
    return (
        <section className="border-t border-ink/10 pt-5">
            <div className="flex items-center gap-2">
                <History aria-hidden className="h-4 w-4 text-ink/60" />
                <h3 className="font-serif text-lg text-ink">檔案事件紀錄</h3>
            </div>
            {events === null ? (
                <p className="mt-3 font-sans text-sm text-copy-muted">載入中…</p>
            ) : events.length === 0 ? (
                <p className="mt-3 font-sans text-sm text-copy-muted">尚無事件紀錄。</p>
            ) : (
                <div className="mt-3 flex flex-col gap-2">
                    {events.map((event) => (
                        <div
                            key={event.id}
                            className="grid gap-1 rounded-[var(--radius-small)] bg-ink/[0.03] px-3 py-2 font-sans text-xs sm:grid-cols-[auto_1fr_auto] sm:items-baseline sm:gap-3"
                        >
                            <span className="font-bold text-ink">{event.action}</span>
                            <span className="text-copy-muted">
                                {event.from_status || "—"} → {event.to_status || "—"}
                                {event.reason ? ` · ${event.reason}` : ""}
                            </span>
                            <time className="text-copy-muted">{formatDate(event.created_at)}</time>
                        </div>
                    ))}
                </div>
            )}
        </section>
    );
}

function ActionReason({
    value,
    onChange,
    label
}: {
    value: string;
    onChange: (value: string) => void;
    label: string;
}) {
    return (
        <label className="flex flex-col gap-1">
            <span className="font-sans text-xs text-copy-muted">{label}</span>
            <textarea
                className="min-h-20 w-full resize-y rounded-[var(--radius-small)] border border-ink/15 bg-surface px-3 py-2 font-sans text-sm text-ink outline-none focus:border-ink/40"
                maxLength={2000}
                value={value}
                onChange={(event) => onChange(event.target.value)}
            />
        </label>
    );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
    return (
        <label className="flex min-w-0 flex-col gap-1">
            <span className="font-sans text-xs text-copy-muted">{label}</span>
            {children}
        </label>
    );
}

function DateField({
    label,
    value,
    onChange
}: {
    label: string;
    value: string;
    onChange: (value: string) => void;
}) {
    return (
        <Field label={label}>
            <input
                className={`${inputClass} w-full`}
                type="date"
                value={dateInputValue(value)}
                onChange={(event) => onChange(event.target.value || "-")}
            />
        </Field>
    );
}

// Plain text, not <input type="time">: the native widget renders 12h/24h per
// browser locale while storing 24h underneath, which confused staff.
function TimeField({
    label,
    value,
    onChange
}: {
    label: string;
    value: string;
    onChange: (value: string) => void;
}) {
    return (
        <Field label={label}>
            <input
                className={`${inputClass} w-full`}
                type="text"
                inputMode="numeric"
                placeholder="14:00"
                maxLength={5}
                value={dateInputValue(value)}
                onChange={(event) => onChange(event.target.value || "-")}
                onBlur={(event) => onChange(normalizeTimeInput(event.target.value))}
            />
        </Field>
    );
}

// "9:0" / "9：00" / " 14:00 " -> "09:00" / "14:00"; unrecognized input is left as typed.
function normalizeTimeInput(raw: string): string {
    const trimmed = raw.trim().replace("：", ":");
    if (!trimmed) return "-";
    const match = /^(\d{1,2}):(\d{1,2})$/.exec(trimmed);
    if (!match) return trimmed;
    const hour = Number(match[1]);
    const minute = Number(match[2]);
    if (hour > 23 || minute > 59) return trimmed;
    return `${String(hour).padStart(2, "0")}:${String(minute).padStart(2, "0")}`;
}

function DetailItem({
    label,
    value,
    mono = false
}: {
    label: string;
    value: string;
    mono?: boolean;
}) {
    return (
        <div className="min-w-0">
            <dt className="text-xs text-copy-muted">{label}</dt>
            <dd className={twMerge("mt-1 break-words text-ink", mono && "font-mono text-xs")}>
                {value || "—"}
            </dd>
        </div>
    );
}

function TextBlock({
    title,
    value,
    children
}: {
    title: string;
    value: string;
    children?: React.ReactNode;
}) {
    return (
        <div className="rounded-[var(--radius-small)] border border-ink/10 p-3">
            <p className="font-sans text-xs text-copy-muted">{title}</p>
            {children ?? (
                <p className="mt-1 font-sans text-sm leading-6 whitespace-pre-wrap text-ink">
                    {value || "—"}
                </p>
            )}
        </div>
    );
}

function statusBadgeClasses(status: string): string {
    return twMerge(
        "inline-flex rounded-full px-3 py-1 font-sans text-xs font-bold",
        (status === "published" || status === "approved") && "bg-accent-green-strong text-ink",
        (status === "pending" || status === "pending_review") && "bg-accent-yellow text-ink",
        (status === "queued" || status === "processing") && "bg-ink/10 text-copy-muted",
        (status === "rejected" || status === "failed") && "bg-red-100 text-red-700",
        status === "archived" && "bg-ink/10 text-copy-muted",
        status === "draft" && "bg-ink/10 text-copy-muted"
    );
}

function StatusBadge({ label, status }: { label: string; status: string }) {
    return <span className={statusBadgeClasses(status)}>{label}</span>;
}

// Same pill sizing as StatusBadge so the two line up. The message itself
// stays out of the row entirely and only opens in a dialog on request,
// instead of stretching the row on every failed upload.
const intakeChannelLabel: Record<string, string> = {
    admin_upload: "管理員上傳",
    ai_system: "AI System 帳號上傳",
    external_api: "外部 AI／搜尋服務"
};

function uploadReportTitle(status: string): string {
    switch (status) {
        case "pending_review":
            return "簡章來源";
        case "queued":
        case "processing":
            return "處理狀態";
        case "failed":
            return "錯誤報告";
        case "rejected":
            return "退回紀錄";
        case "approved":
            return "審核紀錄";
        default:
            return "簡章報告";
    }
}

// One tag, one report: every status tag opens the same dialog, but the body
// shows whatever is actually on record for that status — where a pending
// upload came from, when a finished one was reviewed, or what a failed one's
// error was — instead of only failed uploads being explorable.
function UploadReportDialog({ upload }: { upload: BrochureUpload }) {
    const sourceURL = detectedText(upload.source_url);
    return (
        <Dialog.Root>
            <Dialog.Trigger asChild>
                <button
                    type="button"
                    className={twMerge(
                        statusBadgeClasses(upload.status),
                        "cursor-pointer hover:opacity-80"
                    )}
                >
                    {brochureUploadStatusLabel[upload.status] ?? upload.status}
                </button>
            </Dialog.Trigger>
            <Dialog.Portal>
                <Dialog.Overlay className="fixed inset-0 z-[80] bg-ink/40" />
                <Dialog.Content className="fixed top-1/2 left-1/2 z-[90] max-h-[calc(100vh-4rem)] w-[calc(100vw-2rem)] max-w-lg -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-[var(--radius-panel)] bg-surface p-6 shadow-[var(--shadow-card)] sm:p-8">
                    <div className="flex items-start justify-between gap-4">
                        <div className="min-w-0">
                            <Dialog.Title className="font-serif text-2xl text-ink">
                                {uploadReportTitle(upload.status)}
                            </Dialog.Title>
                            <p className="mt-1 truncate font-mono text-xs text-copy-muted">
                                {upload.original_file_name}
                            </p>
                        </div>
                        <Dialog.Close asChild>
                            <button
                                type="button"
                                aria-label="關閉"
                                className="shrink-0 rounded-[var(--radius-small)] p-1.5 text-copy-muted hover:bg-ink/5 hover:text-ink"
                            >
                                <X aria-hidden className="h-5 w-5" />
                            </button>
                        </Dialog.Close>
                    </div>

                    <dl className="mt-6 grid gap-x-6 gap-y-4 font-sans text-sm sm:grid-cols-2">
                        <DetailItem
                            label="狀態"
                            value={brochureUploadStatusLabel[upload.status] ?? upload.status}
                        />
                        <DetailItem label="上傳時間" value={formatDate(upload.created_at)} />
                        <DetailItem
                            label="來源管道"
                            value={intakeChannelLabel[upload.intake_channel] ?? upload.intake_channel}
                        />
                        {sourceURL ? (
                            <DetailItem label="原始網址" value={sourceURL} mono />
                        ) : null}
                        {upload.detected_academic_year ? (
                            <DetailItem
                                label="系統辨識學年度"
                                value={String(upload.detected_academic_year)}
                            />
                        ) : null}
                        {detectedText(upload.detected_school_code) ? (
                            <DetailItem
                                label="系統辨識學校編號"
                                value={detectedText(upload.detected_school_code)}
                                mono
                            />
                        ) : null}
                        {detectedText(upload.detected_school_name) ? (
                            <DetailItem
                                label="系統辨識學校"
                                value={detectedText(upload.detected_school_name)}
                            />
                        ) : null}
                        {upload.status === "rejected" || upload.status === "approved" ? (
                            <DetailItem
                                label={upload.status === "approved" ? "確認時間" : "退回時間"}
                                value={upload.reviewed_at ? formatDate(upload.reviewed_at) : "—"}
                            />
                        ) : null}
                        {upload.error_code ? (
                            <DetailItem label="錯誤代碼" value={upload.error_code} mono />
                        ) : null}
                    </dl>

                    {upload.status === "queued" || upload.status === "processing" ? (
                        <p className="mt-5 font-sans text-sm text-copy-muted">
                            系統仍在整理文字或執行 OCR，請稍後重新整理查看結果。
                        </p>
                    ) : null}

                    {upload.error_message ? (
                        <div className="mt-5">
                            <p className="font-sans text-xs font-bold text-copy-muted">錯誤內容</p>
                            <pre className="mt-2 max-h-64 overflow-auto rounded-[var(--radius-small)] bg-red-50 p-3 font-mono text-xs whitespace-pre-wrap break-words text-red-700">
                                {upload.error_message}
                            </pre>
                        </div>
                    ) : null}
                </Dialog.Content>
            </Dialog.Portal>
        </Dialog.Root>
    );
}

function Th({ children }: { children?: React.ReactNode }) {
    return (
        <th className="px-4 py-3 font-sans text-xs font-bold tracking-wide uppercase">
            {children}
        </th>
    );
}

function LoadingState() {
    return (
        <div className="flex items-center justify-center gap-2 p-8 font-sans text-sm text-copy-muted">
            <Loader2 aria-hidden className="h-4 w-4 animate-spin" />
            載入中…
        </div>
    );
}

function EmptyState({ text }: { text: string }) {
    return <p className="p-8 text-center font-sans text-sm text-copy-muted">{text}</p>;
}

function parseYear(value: string): number | undefined {
    const trimmed = value.trim();
    if (!trimmed) return undefined;
    const year = Number(trimmed);
    return Number.isInteger(year) ? year : undefined;
}

function validateYearAndSchool(year: string, school: string): string | null {
    if (year.trim() && !/^\d{3}$/.test(year.trim())) return "學年度請輸入三位數，例如 115。";
    if (school.trim() && !/^\d{3}$/.test(school.trim())) return "學校編號請輸入三位數，例如 001。";
    return null;
}

function formatDate(iso: string): string {
    if (!iso || iso === "-") return "—";
    try {
        return new Date(iso).toLocaleString("zh-TW", { dateStyle: "medium", timeStyle: "short" });
    } catch {
        return iso;
    }
}

function formatBytes(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function formatExamWeight(item: AdmissionExamItem): string {
    if (item.weight_percent !== undefined) return `${item.weight_percent}%`;
    if (item.multiplier !== undefined) return `${item.multiplier} 倍`;
    return "—";
}

function programToInput(program: AdminAdmissionProgram): AdmissionProgramInput {
    const pageMatch = program.source_locator.match(/-(\d{1,3})$/);
    return {
        academic_year: program.academic_year,
        school_code: program.school_code,
        program_code: program.program_code,
        admission_program_name: program.admission_program_name,
        admission_quota: program.admission_quota,
        exam_items: program.exam_items,
        timeline_events: program.timeline_events,
        brochure_is_tentative: program.brochure_is_tentative,
        consultation_phone: program.consultation_phone,
        consultation_email: program.consultation_email,
        consultation_contact: program.consultation_contact,
        brochure_url: program.brochure_url,
        special_talent_target: program.special_talent_target,
        different_education_backgrounds: program.different_education_backgrounds,
        different_education_other: program.different_education_other,
        notes: program.notes,
        source_page: pageMatch ? Number(pageMatch[1]) : undefined,
        registration_fee: program.registration_fee,
        exam_location: program.exam_location,
        recommendation_letter_deadline: program.recommendation_letter_deadline,
        portfolio_deadline: program.portfolio_deadline,
        checkin_waitlist_process: program.checkin_waitlist_process,
        fee_reduction_eligibility: program.fee_reduction_eligibility,
        admission_group: program.admission_group,
        cross_group: program.cross_group,
        admission_category: program.admission_category,
        priority_admission: program.priority_admission,
        portfolio_required: program.portfolio_required,
        recommendation_letter_type: program.recommendation_letter_type,
        max_applicable_programs: program.max_applicable_programs,
        applicant_count: program.applicant_count,
        interview_count: program.interview_count,
        admitted_count: program.admitted_count,
        waitlisted_count: program.waitlisted_count,
        admission_rate: program.admission_rate,
        first_stage_pass_rate: program.first_stage_pass_rate,
        competition_ratio: program.competition_ratio,
        school_official_url: program.school_official_url,
        department_official_url: program.department_official_url
    };
}

function candidateToProgramInput(candidate: BrochureUploadCandidate): AdmissionProgramInput {
    const data: Record<string, unknown> = candidate.data;
    const rawExamItems = Array.isArray(data.exam_items) ? data.exam_items : [];
    const sourcePage =
        candidate.source_page && candidate.source_page > 0
            ? candidate.source_page
            : asNumber(data.source_page, 0) || undefined;
    const examItems = rawExamItems.filter(isRecord).map((item, index) => {
        const normalized: AdmissionExamItem = {
            name: asText(item.name, ""),
            stage: asText(item.stage, "-"),
            sort_order: asNumber(item.sort_order, index + 1) || index + 1,
            description: asText(item.description, ""),
            source_page: asText(item.source_page, sourcePage ? String(sourcePage) : "-")
        };
        const weight = asOptionalNumber(item.weight_percent);
        const multiplier = asOptionalNumber(item.multiplier);
        if (weight !== undefined) normalized.weight_percent = weight;
        if (multiplier !== undefined) normalized.multiplier = multiplier;
        return normalized;
    });
    const rawTimelineEvents = Array.isArray(data.timeline_events) ? data.timeline_events : [];
    const timelineEvents = rawTimelineEvents.filter(isRecord).map(
        (event, index): AdmissionTimelineEvent => ({
            name: asText(event.name, ""),
            start_date: asText(event.start_date),
            start_time: asText(event.start_time),
            end_date: asText(event.end_date),
            end_time: asText(event.end_time),
            sort_order: asNumber(event.sort_order, index + 1) || index + 1,
            notes: asText(event.notes)
        })
    );

    return {
        academic_year: asNumber(data.academic_year, 0),
        school_code: asCode(data.school_code),
        program_code: candidate.program_code || asCode(data.program_code),
        admission_program_name: asText(data.admission_program_name, ""),
        admission_quota: asNumber(data.admission_quota, 0),
        exam_items: examItems,
        timeline_events: timelineEvents,
        brochure_is_tentative: data.brochure_is_tentative === true,
        consultation_phone: asText(data.consultation_phone),
        consultation_email: asText(data.consultation_email),
        consultation_contact: asText(data.consultation_contact),
        brochure_url: asText(data.brochure_url),
        special_talent_target: asText(data.special_talent_target),
        different_education_backgrounds: asText(data.different_education_backgrounds),
        different_education_other: asText(data.different_education_other),
        notes: asText(data.notes),
        registration_fee: asText(data.registration_fee),
        exam_location: asText(data.exam_location),
        recommendation_letter_deadline: asText(data.recommendation_letter_deadline),
        portfolio_deadline: asText(data.portfolio_deadline),
        checkin_waitlist_process: asText(data.checkin_waitlist_process),
        fee_reduction_eligibility: asText(data.fee_reduction_eligibility),
        admission_group: asText(data.admission_group),
        cross_group: asText(data.cross_group),
        admission_category: asText(data.admission_category),
        priority_admission: asText(data.priority_admission),
        portfolio_required: asText(data.portfolio_required),
        recommendation_letter_type: asText(data.recommendation_letter_type),
        max_applicable_programs: asText(data.max_applicable_programs),
        applicant_count: asText(data.applicant_count),
        interview_count: asText(data.interview_count),
        admitted_count: asText(data.admitted_count),
        waitlisted_count: asText(data.waitlisted_count),
        admission_rate: asText(data.admission_rate),
        first_stage_pass_rate: asText(data.first_stage_pass_rate),
        competition_ratio: asText(data.competition_ratio),
        school_official_url: asText(data.school_official_url),
        department_official_url: asText(data.department_official_url),
        ...(sourcePage ? { source_page: sourcePage } : {})
    };
}

// Standard 招生時程 milestones (大表's 13 columns plus 簡章公告), pre-filled with
// "-" so a new program starts with every standard slot ready to fill in.
const DEFAULT_TIMELINE_MILESTONES = [
    "簡章公告",
    "網路報名",
    "上傳指定繳交資料",
    "特殊身分考生上傳證明期限",
    "推薦函截止",
    "二階（複試）名單公布",
    "二階（面試）",
    "錄取放榜",
    "成績複查申請期限",
    "正取生報到",
    "備取生報到",
    "開始遞補",
    "已報到生放棄資格截止",
    "備取生遞補作業截止"
];

function defaultTimelineEvents(): AdmissionTimelineEvent[] {
    return DEFAULT_TIMELINE_MILESTONES.map((name, index) => ({
        name,
        start_date: "-",
        start_time: "-",
        end_date: "-",
        end_time: "-",
        sort_order: index + 1,
        notes: "-"
    }));
}

function emptyProgramInput(academicYear: number, schoolCode: string): AdmissionProgramInput {
    return {
        academic_year: academicYear,
        school_code: schoolCode,
        program_code: "",
        admission_program_name: "",
        admission_quota: 0,
        exam_items: [],
        timeline_events: defaultTimelineEvents(),
        brochure_is_tentative: false,
        consultation_phone: "-",
        consultation_email: "-",
        consultation_contact: "-",
        brochure_url: "-",
        special_talent_target: "-",
        different_education_backgrounds: "-",
        different_education_other: "-",
        notes: "-",
        registration_fee: "-",
        exam_location: "-",
        recommendation_letter_deadline: "-",
        portfolio_deadline: "-",
        checkin_waitlist_process: "-",
        fee_reduction_eligibility: "-",
        admission_group: "-",
        cross_group: "-",
        admission_category: "-",
        priority_admission: "-",
        portfolio_required: "-",
        recommendation_letter_type: "-",
        max_applicable_programs: "-",
        applicant_count: "-",
        interview_count: "-",
        admitted_count: "-",
        waitlisted_count: "-",
        admission_rate: "-",
        first_stage_pass_rate: "-",
        competition_ratio: "-",
        school_official_url: "-",
        department_official_url: "-"
    };
}

function isRecord(value: unknown): value is Record<string, unknown> {
    return typeof value === "object" && value !== null && !Array.isArray(value);
}

function asText(value: unknown, fallback = "-"): string {
    if (typeof value === "string" && value.trim() && value.trim() !== "null") {
        return value.trim();
    }
    if (typeof value === "number" && Number.isFinite(value)) return String(value);
    return fallback;
}

function displayFormValue(value: string): string {
    return value === "-" ? "" : value;
}

// Backend uses "-" as its missing-value marker, not "", so a plain `||` won't catch it.
function detectedText(value?: string): string {
    return value && value !== "-" ? value : "";
}

function dateInputValue(value: string): string {
    return value === "-" ? "" : value;
}

function asCode(value: unknown): string {
    const text = asText(value, "");
    return text === "-" ? "" : text;
}

function asNumber(value: unknown, fallback: number): number {
    if (typeof value === "number" && Number.isFinite(value)) return value;
    if (typeof value === "string" && value.trim()) {
        const parsed = Number(value.trim());
        if (Number.isFinite(parsed)) return parsed;
    }
    return fallback;
}

function asOptionalNumber(value: unknown): number | undefined {
    if (value === null || value === undefined || value === "") return undefined;
    const parsed = asNumber(value, Number.NaN);
    return Number.isFinite(parsed) ? parsed : undefined;
}

function optionalNumber(value: string): number | undefined {
    const trimmed = value.trim();
    if (!trimmed) return undefined;
    const parsed = Number(trimmed);
    return Number.isFinite(parsed) ? parsed : undefined;
}

function formatConfidence(value?: number): string {
    return value === undefined ? "—" : `${Math.round(value * 100)}%`;
}
