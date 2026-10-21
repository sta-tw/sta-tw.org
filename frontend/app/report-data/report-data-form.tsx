"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { FileUp, Plus, Trash2, X } from "lucide-react";
import { Label } from "radix-ui";
import TurnstileWidget from "../components/turnstile-widget";
import {
    listPublicContributors,
    submitAdmissionsContribution,
    type ContributionCategory,
    type PublicContributor
} from "../lib/api/contributions";
import { ApiError } from "../lib/api/types";

const inputClass =
    "w-full rounded-md border border-ink/20 bg-transparent px-3 py-2.5 font-sans text-base text-ink outline-none transition-colors placeholder:text-copy-muted focus:border-ink/50 focus:ring-2 focus:ring-ink/10";
const focusClass =
    "focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ink";
const maxFileBytes = 10 * 1024 * 1024;
const maxUploadFiles = 5;
const maxContributionRows = 100;
const latestAcademicYear = Math.max(116, new Date().getFullYear() - 1911);
const academicYears = Array.from({ length: latestAcademicYear - 103 }, (_, index) => index + 104);

type ProgramCountRow = {
    id: number;
    programName: string;
    peopleCount: string;
};

const categories: { value: ContributionCategory; label: string }[] = [
    {
        value: "applicants",
        label: "應屆報名人數"
    },
    {
        value: "second_stage",
        label: "進入二階人數"
    },
    {
        value: "waitlist",
        label: "應屆遞補人數"
    },
    {
        value: "historical_brochure",
        label: "歷屆簡章"
    }
];

function contributionErrorMessage(cause: unknown): string {
    if (cause instanceof ApiError) {
        switch (cause.code) {
            case "invalid_contribution":
                return "資料格式不完整，請檢查學年度、聯絡信箱和必填欄位。人數資料請擇一手動填寫或上傳。";
            case "invalid_file":
                return "檔案格式不支援，或超過 5 個／單檔 10 MB 的限制。";
            case "malware_detected":
                return "附件未通過安全檢查，請更換檔案後再送出。";
            case "rate_limited":
                return "送出次數過多，請稍後再試。";
            case "turnstile_invalid":
                return "Cloudflare 驗證已失效，請重新完成驗證後送出。";
            case "turnstile_unavailable":
                return "驗證服務暫時無法使用，請稍後再試。";
            case "scan_unavailable":
            case "storage_unavailable":
                return "目前無法處理附件，請稍後再試，或改提供簡章連結。";
            case "network_error":
                return cause.message;
            default:
                return "送出失敗，請稍後再試。";
        }
    }
    return "送出失敗，請稍後再試。";
}

export default function ReportDataForm() {
    const [category, setCategory] = useState<ContributionCategory>("applicants");
    const [yearChoice, setYearChoice] = useState(String(latestAcademicYear));
    const [fileName, setFileName] = useState("");
    const [nickname, setNickname] = useState("");
    const [publishNickname, setPublishNickname] = useState(false);
    const [submitting, setSubmitting] = useState(false);
    const [submitted, setSubmitted] = useState(false);
    const [emailQueued, setEmailQueued] = useState(false);
    const [forumPosted, setForumPosted] = useState(false);
    const [notice, setNotice] = useState("");
    const [dataInputMode, setDataInputMode] = useState<"manual" | "upload">("manual");
    const [programRows, setProgramRows] = useState<ProgramCountRow[]>([
        { id: 1, programName: "", peopleCount: "" }
    ]);
    const [uploadedFiles, setUploadedFiles] = useState<File[]>([]);
    const [contributors, setContributors] = useState<PublicContributor[]>([]);
    const [turnstileToken, setTurnstileToken] = useState("");
    const [resetSignal, setResetSignal] = useState(0);
    const nextProgramRowID = useRef(2);

    useEffect(() => {
        const controller = new AbortController();
        listPublicContributors(controller.signal)
            .then((response) => setContributors(response.data))
            .catch(() => {
                // The form stays available when the public thank-you list is unavailable.
            });
        return () => controller.abort();
    }, []);

    async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
        event.preventDefault();
        if (submitting) return;
        if (!turnstileToken) {
            setNotice("請先完成 Cloudflare 驗證。");
            return;
        }
        const form = event.currentTarget;
        const formData = new FormData(form);
        const selectedFile = formData.get("file");
        const brochureFile = selectedFile instanceof File && selectedFile.size > 0 ? selectedFile : null;
        const hasSelectedFile = category === "historical_brochure"
            ? brochureFile !== null
            : dataInputMode === "upload" && uploadedFiles.length > 0;

        if (brochureFile && brochureFile.size > maxFileBytes) {
            setNotice("PDF 檔案大小不能超過 10 MB。");
            return;
        }
        if (
            category === "historical_brochure" &&
            !hasSelectedFile &&
            !String(formData.get("reference_url") ?? "").trim()
        ) {
            setNotice("請上傳歷屆簡章 PDF，或提供可檢閱的連結。");
            return;
        }

        if (category !== "historical_brochure" && dataInputMode === "upload") {
            if (uploadedFiles.length === 0) {
                setNotice("請選擇要上傳的資料檔。");
                return;
            }
            if (uploadedFiles.length > maxUploadFiles) {
                setNotice(`最多可上傳 ${maxUploadFiles} 個檔案。`);
                return;
            }
            if (uploadedFiles.some((file) => file.size > maxFileBytes)) {
                setNotice("每個檔案大小不能超過 10 MB，請移除超過上限的檔案。");
                return;
            }
            formData.delete("file");
            for (const file of uploadedFiles) formData.append("file", file, file.name);
            formData.delete("data_text");
            formData.delete("program_name");
            formData.delete("people_count");
        } else if (category !== "historical_brochure") {
            const invalidRow = programRows.findIndex((row) => {
                const countText = row.peopleCount.trim();
                const people = Number(countText);
                return (
                    !row.programName.trim() ||
                    !/^\d+$/.test(countText) ||
                    !Number.isSafeInteger(people) ||
                    people > 1_000_000_000
                );
            });
            if (invalidRow !== -1) {
                setNotice(`第 ${invalidRow + 1} 列請填寫科系名稱和 0 到 1,000,000,000 之間的人數。`);
                return;
            }
            if (programRows.length > maxContributionRows) {
                setNotice(`資料最多可新增 ${maxContributionRows} 列。`);
                return;
            }
            formData.delete("file");
            formData.delete("data_text");
            formData.delete("program_name");
            formData.delete("people_count");
            for (const row of programRows) {
                formData.append("program_name", row.programName.trim());
                formData.append("people_count", row.peopleCount.trim());
            }
        }

        if (!hasSelectedFile) formData.delete("file");
        formData.set("academic_year", yearChoice);
        formData.set("category", category);
        formData.set("publish_nickname", publishNickname && nickname.trim() ? "true" : "false");
        formData.set("turnstile_token", turnstileToken);
        setNotice("");
        setSubmitting(true);
        try {
            const response = await submitAdmissionsContribution(formData);
            setEmailQueued(response.data.email_queued === true);
            setForumPosted(response.data.forum_posted === true);
            setSubmitted(true);
        } catch (cause) {
            setNotice(contributionErrorMessage(cause));
            setResetSignal((value) => value + 1);
        } finally {
            setSubmitting(false);
        }
    }

    if (submitted) {
        return (
            <main className="flex flex-1 border-y border-ink/10 bg-background px-5 py-10 sm:px-6 sm:py-14">
                <section className="mx-auto w-full max-w-4xl self-start border-l-4 border-l-accent-green-strong px-5 py-5 sm:px-7 sm:py-6">
                    <h1 className="font-serif text-2xl text-ink sm:text-3xl">資料已送出</h1>
                    <p className="mt-2 font-sans leading-7 text-copy-muted">
                        感謝你協助提供特殊選才資料。聯絡信箱不會公開。
                    </p>
                    <p className="mt-2 font-sans text-sm leading-6 text-copy-muted">
                        {emailQueued
                            ? "回報內容副本已排入寄送，會寄到你填寫的聯絡信箱。"
                            : "回報已保存；目前無法確認副本寄送狀態，請保留你填寫的資料。"}
                    </p>
                    <p className="mt-1 font-sans text-sm leading-6 text-copy-muted">
                        {forumPosted
                            ? "回報也已同步建立在簡章回報討論區。"
                            : "回報已保存，本站管理員仍可在後台檢視。"}
                    </p>
                    {publishNickname && nickname.trim() ? (
                        <p className="mt-2 font-sans text-sm text-copy-muted">
                            你同意公開的暱稱會在資料採用後列入感謝名單。
                        </p>
                    ) : null}
                    <Link
                        href="/"
                        className={`mt-4 inline-flex font-sans font-bold text-ink underline underline-offset-4 ${focusClass}`}
                    >
                        回到首頁
                    </Link>
                </section>
            </main>
        );
    }

    const isCountContribution = category !== "historical_brochure";

    function addProgramRow() {
        if (programRows.length >= maxContributionRows) return;
        const id = nextProgramRowID.current++;
        setProgramRows((rows) => [...rows, { id, programName: "", peopleCount: "" }]);
    }

    return (
        <main className="flex-1 border-y border-ink/10 bg-background">
            <div className="mx-auto w-full max-w-4xl px-5 py-9 sm:px-8 sm:py-12">
                <header className="mb-5 border-b border-ink/10 pb-5">
                    <h1 className="font-serif text-3xl leading-tight text-ink sm:text-4xl">
                        招生資料回報
                    </h1>
                    <p className="mt-2 font-sans text-sm leading-6 text-copy-muted sm:text-base">
                        回報招生人數，或提供歷屆簡章。
                    </p>
                </header>

                <div>
                    <section
                        className="mb-6 border-l-2 border-l-accent-yellow-strong bg-accent-yellow-light/15 px-4 py-3"
                        aria-labelledby="contribution-help"
                    >
                        <p id="contribution-help" className="font-sans text-sm leading-6 text-ink/80">
                            避免重複回報，請先
                            <Link
                                href="/report-data/records"
                                className={`mx-1 font-bold text-ink underline underline-offset-4 ${focusClass}`}
                            >
                                查看資料清單
                            </Link>。
                            每次回報一間學校；人數資料可手動填寫或上傳檔案，擇一即可。
                        </p>
                    </section>
                    <form
                        onSubmit={handleSubmit}
                        className="divide-y divide-ink/10 border-y border-ink/15"
                    >
                        <section className="grid gap-5 px-4 py-5 sm:px-6 sm:py-6">
                            <h2 className="font-sans text-base font-bold text-ink">基本資料</h2>
                            <Field label="資料所屬學年度" htmlFor="contribution-year" required>
                                <select
                                    id="contribution-year"
                                    name="academic_year"
                                    value={yearChoice}
                                    onChange={(event) => setYearChoice(event.target.value)}
                                    className={inputClass}
                                >
                                    {academicYears.map((year) => (
                                        <option key={year} value={year}>
                                            {year}
                                        </option>
                                    ))}
                                </select>
                            </Field>
                            <Field label="提供資料的學校" htmlFor="contribution-school" required>
                                <input
                                    id="contribution-school"
                                    name="school_name"
                                    required
                                    maxLength={240}
                                    className={inputClass}
                                    placeholder="請寫學校全名，例如：國立臺灣大學"
                                />
                                <span className="mt-1.5 block font-sans text-sm text-copy-muted">
                                    不同學校請分次提交。
                                </span>
                            </Field>
                        </section>

                        <section className="px-4 py-5 sm:px-6 sm:py-6">
                            <fieldset className="border-0 p-0">
                                <legend className="font-sans text-base font-bold text-ink sm:text-lg">
                                    資料類別 <span className="text-red-700" aria-hidden="true">*</span>
                                </legend>
                                <div className="mt-3 divide-y divide-ink/10 border-y border-ink/10">
                                    {categories.map((option) => (
                                        <label
                                            key={option.value}
                                            className={
                                                "flex min-w-0 cursor-pointer items-center gap-3 border-l-4 px-3 py-3 transition-colors " +
                                                (category === option.value
                                                    ? "border-l-accent-green-strong bg-accent-green/20"
                                                    : "border-l-transparent hover:bg-accent-green/10")
                                            }
                                        >
                                            <input
                                                type="radio"
                                                name="category_choice"
                                                value={option.value}
                                                checked={category === option.value}
                                                onChange={() => {
                                                    setCategory(option.value);
                                                    setFileName("");
                                                    setUploadedFiles([]);
                                                }}
                                                className="mt-1 size-4 shrink-0 accent-ink"
                                            />
                                            <span className="min-w-0 flex-1 font-sans font-bold text-ink">
                                                {option.label}
                                            </span>
                                        </label>
                                    ))}
                                </div>
                            </fieldset>
                        </section>

                        {isCountContribution ? (
                            <section className="grid gap-5 px-4 py-5 sm:px-6 sm:py-6">
                                <fieldset className="min-w-0 border-0 p-0">
                                    <legend className="font-sans text-base font-bold text-ink sm:text-lg">
                                        資料提供方式 <span className="text-red-700" aria-hidden="true">*</span>
                                    </legend>
                                    <div className="mt-3 divide-y divide-ink/10 border-y border-ink/10">
                                        <label className={`flex cursor-pointer items-center gap-3 border-l-4 px-3 py-3 font-sans text-sm font-bold text-ink transition-colors ${dataInputMode === "manual" ? "border-l-accent-green-strong bg-accent-green/20" : "border-l-transparent hover:bg-accent-green/10"}`}>
                                            <input
                                                type="radio"
                                                name="data_input_mode"
                                                value="manual"
                                                checked={dataInputMode === "manual"}
                                                onChange={() => setDataInputMode("manual")}
                                                className="size-4 accent-ink"
                                            />
                                            手動填寫科系與人數
                                        </label>
                                        <label className={`flex cursor-pointer items-center gap-3 border-l-4 px-3 py-3 font-sans text-sm font-bold text-ink transition-colors ${dataInputMode === "upload" ? "border-l-accent-green-strong bg-accent-green/20" : "border-l-transparent hover:bg-accent-green/10"}`}>
                                            <input
                                                type="radio"
                                                name="data_input_mode"
                                                value="upload"
                                                checked={dataInputMode === "upload"}
                                                onChange={() => setDataInputMode("upload")}
                                                className="size-4 accent-ink"
                                            />
                                            上傳資料檔
                                        </label>
                                    </div>
                                    {dataInputMode === "manual" ? (
                                        <>
                                            <p className="mt-3 font-sans text-sm leading-6 text-copy-muted">
                                                科系名稱請填「系」前的文字，人數請填整數。
                                            </p>
                                            <div className="mt-3 grid gap-2">
                                                {programRows.map((row, index) => (
                                                    <div
                                                        key={row.id}
                                                        className="grid grid-cols-[minmax(0,1fr)_1rem_5rem_1rem_2.5rem] items-center gap-1 border-b border-ink/10 py-2 sm:grid-cols-[minmax(0,1fr)_1.25rem_7rem_1.25rem_2.75rem] sm:gap-2 sm:py-2.5"
                                                    >
                                                        <label className="sr-only" htmlFor={`contribution-program-${row.id}`}>
                                                            第 {index + 1} 列科系名稱
                                                        </label>
                                                        <input
                                                            id={`contribution-program-${row.id}`}
                                                            type="text"
                                                            required
                                                            maxLength={240}
                                                            value={row.programName}
                                                            onChange={(event) => {
                                                                const programName = event.target.value;
                                                                setProgramRows((rows) => rows.map((entry) =>
                                                                    entry.id === row.id ? { ...entry, programName } : entry
                                                                ));
                                                            }}
                                                            className={inputClass}
                                                            placeholder="科系名稱（不含「系」）"
                                                        />
                                                        <span className="text-center font-sans text-sm font-bold text-copy-muted">系</span>
                                                        <label className="sr-only" htmlFor={`contribution-people-${row.id}`}>
                                                            第 {index + 1} 列人數
                                                        </label>
                                                        <input
                                                            id={`contribution-people-${row.id}`}
                                                            type="number"
                                                            required
                                                            min={0}
                                                            max={1_000_000_000}
                                                            step={1}
                                                            value={row.peopleCount}
                                                            onChange={(event) => {
                                                                const peopleCount = event.target.value;
                                                                setProgramRows((rows) => rows.map((entry) =>
                                                                    entry.id === row.id ? { ...entry, peopleCount } : entry
                                                                ));
                                                            }}
                                                            className={`${inputClass} px-2 text-center sm:px-3`}
                                                            aria-label={`第 ${index + 1} 列人數`}
                                                        />
                                                        <span className="text-center font-sans text-sm font-bold text-copy-muted">人</span>
                                                        <button
                                                            type="button"
                                                            onClick={() => setProgramRows((rows) => rows.length > 1 ? rows.filter((entry) => entry.id !== row.id) : rows)}
                                                            disabled={programRows.length === 1}
                                                            aria-label={`刪除第 ${index + 1} 列`}
                                                            className={`inline-flex size-10 items-center justify-center text-ink/65 transition-colors hover:text-red-700 disabled:cursor-not-allowed disabled:opacity-35 ${focusClass}`}
                                                        >
                                                            <Trash2 aria-hidden className="size-4" />
                                                        </button>
                                                    </div>
                                                ))}
                                            </div>
                                            <button
                                                type="button"
                                                onClick={addProgramRow}
                                                disabled={programRows.length >= maxContributionRows}
                                                className={`mt-3 inline-flex h-10 items-center gap-2 border-b-2 border-accent-green-strong px-1 font-sans text-sm font-bold text-ink transition-colors hover:bg-accent-green/20 disabled:cursor-not-allowed disabled:opacity-50 ${focusClass}`}
                                            >
                                                <Plus aria-hidden className="size-4" />
                                                新增
                                            </button>
                                        </>
                                    ) : (
                                        <MultiFileInput files={uploadedFiles} onFilesChange={setUploadedFiles} />
                                    )}
                                </fieldset>
                            </section>
                        ) : (
                            <section className="grid gap-5 px-4 py-5 sm:px-6 sm:py-6">
                                <h2 className="font-sans text-base font-bold text-ink">簡章資料</h2>
                                <Field label="上傳簡章 PDF" htmlFor="contribution-file">
                                    <FileInput fileName={fileName} onFileNameChange={setFileName} />
                                    <span className="mt-2 block font-sans text-sm text-copy-muted">
                                        檔案上限 10 MB。也可以只提供下方連結。
                                    </span>
                                </Field>
                                <Field label="或提供可檢閱的連結" htmlFor="contribution-url">
                                    <input
                                        id="contribution-url"
                                        name="reference_url"
                                        type="url"
                                        maxLength={2048}
                                        required={!fileName}
                                        className={inputClass}
                                        placeholder="https://..."
                                    />
                                </Field>
                            </section>
                        )}

                        <section className="grid gap-5 px-4 py-5 sm:px-6 sm:py-6">
                            <h2 className="font-sans text-base font-bold text-ink">聯絡方式</h2>
                            <Field label="聯絡信箱" htmlFor="contribution-email" required>
                                <input
                                    id="contribution-email"
                                    name="email"
                                    type="email"
                                    autoComplete="email"
                                    required
                                    maxLength={254}
                                    className={inputClass}
                                    placeholder="僅供本站管理員聯絡與寄送回報副本"
                                />
                            </Field>
                            <Field label="暱稱（選填）" htmlFor="contribution-nickname">
                                <input
                                    id="contribution-nickname"
                                    name="nickname"
                                    value={nickname}
                                    onChange={(event) => setNickname(event.target.value)}
                                    maxLength={120}
                                    className={inputClass}
                                    placeholder="可列入網站感謝名單"
                                />
                            </Field>
                            <label className="flex items-start gap-3 font-sans text-sm leading-6 text-ink/75">
                                <input
                                    type="checkbox"
                                    checked={publishNickname}
                                    onChange={(event) => setPublishNickname(event.target.checked)}
                                    disabled={!nickname.trim()}
                                    className="mt-1 size-4 shrink-0 accent-ink"
                                />
                                <span>我同意本站在資料採用後，將此暱稱公開於感謝名單。</span>
                            </label>
                        </section>

                        <section className="grid gap-5 px-4 py-5 sm:px-6 sm:py-6">
                            <label
                                className="absolute -left-[10000px] h-px w-px overflow-hidden"
                                aria-hidden="true"
                            >
                                網站
                                <input name="website" type="text" tabIndex={-1} autoComplete="off" />
                            </label>

                            {notice ? (
                                <p
                                    role="alert"
                                    className="border-l-2 border-red-700 bg-red-50/40 px-3 py-2.5 font-sans text-sm text-red-800"
                                >
                                    {notice}
                                </p>
                            ) : null}

                            <TurnstileWidget
                                action="admissions_contribution"
                                onTokenChange={setTurnstileToken}
                                resetSignal={resetSignal}
                            />

                            <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                                <p className="font-sans text-sm leading-6 text-copy-muted">
                                    聯絡信箱不會公開；回報副本會寄到此信箱，PDF 不會附在確認信中。
                                </p>
                                <button
                                    type="submit"
                                    disabled={submitting || !turnstileToken}
                                    className={`inline-flex h-11 w-full shrink-0 items-center justify-center bg-button px-5 font-sans text-base font-bold text-button-foreground transition-colors hover:bg-button-hover disabled:cursor-not-allowed disabled:opacity-60 sm:w-auto ${focusClass}`}
                                >
                                    {submitting ? "送出中…" : "送出資料"}
                                </button>
                            </div>
                        </section>
                    </form>

                    {contributors.length > 0 ? (
                        <section className="mt-8 border-t border-ink/10 pt-6" aria-labelledby="contribution-thanks">
                            <h3
                                id="contribution-thanks"
                                className="font-sans text-base font-bold text-ink"
                            >
                                感謝資料提供者
                            </h3>
                            <ul className="mt-2 flex flex-wrap gap-x-4 gap-y-1">
                                {contributors.map((contributor, index) => (
                                    <li
                                        key={`${contributor.nickname}-${contributor.reviewed_at}-${index}`}
                                        className="font-sans text-sm text-copy-muted"
                                    >
                                        {contributor.nickname}
                                    </li>
                                ))}
                            </ul>
                        </section>
                    ) : null}
                </div>
            </div>
        </main>
    );
}

function Field({
    label,
    htmlFor,
    required = false,
    children
}: {
    label: string;
    htmlFor: string;
    required?: boolean;
    children: React.ReactNode;
}) {
    return (
        <div className="min-w-0">
            <Label.Root
                htmlFor={htmlFor}
                className="mb-2 block font-sans text-sm font-bold text-ink sm:text-base"
            >
                {label}
                {required ? (
                    <span className="ml-1 text-red-700" aria-hidden="true">
                        *
                    </span>
                ) : null}
            </Label.Root>
            {children}
        </div>
    );
}

function FileInput({
    fileName,
    onFileNameChange
}: {
    fileName: string;
    onFileNameChange: (value: string) => void;
}) {
    return (
        <label className="inline-flex min-h-10 max-w-full cursor-pointer items-center gap-2 border-b border-ink/25 py-2 font-sans text-sm font-bold text-ink transition-colors hover:border-ink/60">
            <FileUp aria-hidden className="size-4 shrink-0 text-ink/65" />
            <span className="min-w-0 break-all">
                {fileName || "選擇 PDF 檔案"}
            </span>
            <input
                id="contribution-file"
                name="file"
                type="file"
                accept="application/pdf,.pdf"
                onChange={(event) => onFileNameChange(event.target.files?.[0]?.name ?? "")}
                className="sr-only"
            />
        </label>
    );
}

function MultiFileInput({
    files,
    onFilesChange
}: {
    files: File[];
    onFilesChange: (files: File[]) => void;
}) {
    return (
        <div className="mt-3">
            <label className="inline-flex min-h-10 max-w-full cursor-pointer items-center gap-2 border-b border-ink/25 py-2 font-sans text-sm font-bold text-ink transition-colors hover:border-ink/60">
                <FileUp aria-hidden className="size-4 shrink-0 text-ink/65" />
                <span className="min-w-0 break-words">
                    {files.length > 0 ? `已選擇 ${files.length} 個檔案` : "選擇資料檔"}
                </span>
                <input
                    type="file"
                    multiple
                    accept=".pdf,.csv,.tsv,.txt,.json,.doc,.docx,.ppt,.pptx,.xls,.xlsx,.jpg,.jpeg,.png,.gif,.webp"
                    onChange={(event) => {
                        onFilesChange(Array.from(event.currentTarget.files ?? []));
                        event.currentTarget.value = "";
                    }}
                    className="sr-only"
                />
            </label>
            {files.length > 0 ? (
                <ul className="mt-3 grid gap-2">
                    {files.map((file, index) => (
                        <li
                            key={`${file.name}-${file.size}-${file.lastModified}-${index}`}
                            className="flex min-w-0 items-center justify-between gap-3 border-b border-ink/10 py-2"
                        >
                            <span className="min-w-0 break-all font-sans text-sm text-ink">
                                {file.name}
                                <span className="ml-2 whitespace-nowrap text-xs text-copy-muted">
                                    {(file.size / (1024 * 1024)).toFixed(1)} MB
                                </span>
                            </span>
                            <button
                                type="button"
                                onClick={() => onFilesChange(files.filter((_, fileIndex) => fileIndex !== index))}
                                aria-label={`移除 ${file.name}`}
                                className={`inline-flex size-9 shrink-0 items-center justify-center text-ink/65 transition-colors hover:text-red-700 ${focusClass}`}
                            >
                                <X aria-hidden className="size-4" />
                            </button>
                        </li>
                    ))}
                </ul>
            ) : null}
        </div>
    );
}
