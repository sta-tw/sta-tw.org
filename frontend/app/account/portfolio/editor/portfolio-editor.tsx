"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { FileText, Plus, Trash2 } from "lucide-react";
import { useCallback, useEffect, useId, useMemo, useState } from "react";
import Button from "../../../components/button";
import Field, { FieldGroup } from "../../../components/field";
import FileUpload from "../../../components/file-upload";
import IconButton from "../../../components/icon-button";
import { Input } from "../../../components/input";
import RadioGroup from "../../../components/radio-group";
import SchoolPicker, {
    findSchoolByName,
    useSchoolDirectory
} from "../../../components/school-picker";
import Select from "../../../components/select";
import { ErrorText, Resource } from "../../../components/status";
import { listAdmissionPrograms } from "../../../lib/api/admissions";
import {
    getMyPortfolio,
    PORTFOLIO_FILE_ACCEPT,
    PORTFOLIO_FILE_MAX_BYTES,
    PORTFOLIO_ITEM_LIMIT,
    PORTFOLIO_ITEM_NAME_MAX_LENGTH,
    savePortfolio,
    submitPortfolio,
    type Portfolio,
    type PortfolioItem,
    type PortfolioVisibility
} from "../../../lib/api/portfolio";
import { useCurrentAccount } from "../../../lib/current-account";
import { currentRocYear, formatBytes, formatDateTime, rocYearOptions } from "../../../lib/format";
import { useAction, useResource } from "../../../lib/use-resource";
import AccountPage from "../../account-page";
import SubmitReviewDialog from "../submit-review-dialog";

const OVERVIEW_HREF = "/account/portfolio/";
const maxSizeLabel = formatBytes(PORTFOLIO_FILE_MAX_BYTES);

// Seniors share past years too, so offer a few years back.
const yearOptions = rocYearOptions({
    from: currentRocYear() - 5,
    to: currentRocYear() + 1,
    suffix: " 學年度"
});

type DraftItem = {
    /** Stable React key; equals the saved item id for existing items. */
    key: string;
    existing?: PortfolioItem;
    name: string;
    file?: File;
};

const blankItem = (): DraftItem => ({ key: crypto.randomUUID(), name: "" });

export default function PortfolioEditor() {
    const id = useSearchParams().get("id")?.trim() ?? "";
    const load = useCallback(
        (signal: AbortSignal) =>
            id ? getMyPortfolio(id, { signal }).then(({ data }) => data) : Promise.resolve(null),
        [id]
    );
    const { state, reload } = useResource(load);

    return (
        <AccountPage
            title={id ? "編輯備審" : "新增備審"}
            description="一份備審對應一個申請校系，可以放入多個 PDF 檔案。"
        >
            <Resource state={state} reload={reload}>
                {(portfolio) => <EditorForm key={portfolio?.id ?? "new"} portfolio={portfolio} />}
            </Resource>
        </AccountPage>
    );
}

function EditorForm({ portfolio }: { portfolio: Portfolio | null }) {
    const router = useRouter();
    const { me } = useCurrentAccount();
    const directory = useSchoolDirectory();
    const [year, setYear] = useState(portfolio ? String(portfolio.academic_year) : "");
    const [school, setSchool] = useState(portfolio?.school_name ?? "");
    const [program, setProgram] = useState(portfolio?.program_name ?? "");
    const [visibility, setVisibility] = useState<PortfolioVisibility>(
        portfolio?.visibility ?? "private"
    );
    const [items, setItems] = useState<DraftItem[]>(
        portfolio?.items.map((item) => ({ key: item.id, existing: item, name: item.name })) ?? [
            blankItem()
        ]
    );
    const [showErrors, setShowErrors] = useState(false);
    const { pending, error, setError, run } = useAction();
    const saving = pending !== null;

    const verified = me ? me.account.identity_status !== "temporary" : false;
    const programs = usePrograms(findSchoolByName(directory.schools, school)?.school_code);

    const errors = {
        year: year ? null : "請選擇申請學年度。",
        school: school.trim() ? null : "請輸入學校名稱。",
        program: program.trim() ? null : "請輸入系所名稱。",
        items: items.map((item) => ({
            name: item.name.trim() ? null : "請輸入項目名稱。",
            file: item.file || item.existing ? null : "請上傳 PDF 檔案。"
        }))
    };
    const valid =
        !errors.year &&
        !errors.school &&
        !errors.program &&
        errors.items.every((item) => !item.name && !item.file);
    const shown = (message: string | null) => (showErrors ? message : null);

    const updateItem = (key: string, update: Partial<DraftItem>) =>
        setItems((current) =>
            current.map((item) => (item.key === key ? { ...item, ...update } : item))
        );

    function pickFile(key: string, file: File) {
        const isPdf = file.type === "application/pdf" || /\.pdf$/i.test(file.name);
        if (!isPdf) return setError("只能上傳 PDF 檔案。");
        if (file.size > PORTFOLIO_FILE_MAX_BYTES) {
            return setError(`檔案超過 ${maxSizeLabel} 上限。`);
        }
        setError(null);
        updateItem(key, { file });
    }

    function save(submit: boolean) {
        setShowErrors(true);
        if (!valid) return;
        void run(async () => {
            const { data } = await savePortfolio(
                {
                    academic_year: Number(year),
                    school_name: school,
                    program_name: program,
                    visibility,
                    items: items.map((item) => ({
                        id: item.existing?.id,
                        name: item.name,
                        file: item.file
                    }))
                },
                portfolio?.id
            );
            if (submit) await submitPortfolio(data.id);
            router.push(OVERVIEW_HREF);
        });
    }

    const shared = visibility !== "private";
    const reviewed = portfolio?.review_status === "approved" && shared;

    return (
        <form
            noValidate
            className="flex flex-col gap-8"
            onSubmit={(event) => {
                event.preventDefault();
                save(false);
            }}
        >
            <section aria-label="申請校系" className="flex flex-col gap-5">
                <div className="grid gap-5 sm:grid-cols-[12rem_minmax(0,1fr)]">
                    <Field label="申請學年度" required error={shown(errors.year)}>
                        <Select
                            value={year}
                            placeholder="請選擇"
                            options={yearOptions}
                            onChange={(event) => setYear(event.target.value)}
                        />
                    </Field>
                    <Field
                        label="學校名稱"
                        required
                        error={shown(errors.school ?? directory.error)}
                    >
                        <SchoolPicker directory={directory} value={school} onChange={setSchool} />
                    </Field>
                </div>
                <Field
                    label="系所名稱"
                    required
                    hint={programs.length > 0 ? "可從建議中選擇，或直接輸入。" : undefined}
                    error={shown(errors.program)}
                >
                    <ProgramInput value={program} onChange={setProgram} suggestions={programs} />
                </Field>
            </section>

            {/* The divider is on a wrapper: on the fieldset it would cross the legend. */}
            <div className="border-t border-ink/10 pt-6">
                <FieldGroup label="分享範圍" required>
                    <RadioGroup
                        value={visibility}
                        onValueChange={(value) => setVisibility(value as PortfolioVisibility)}
                        options={[
                            {
                                value: "private",
                                label: "私人",
                                description: "只有自己看得到，不需要審核。"
                            },
                            {
                                value: "exchange",
                                label: "交換備審",
                                disabled: !verified,
                                description: verified ? (
                                    "僅限已驗證的特選生瀏覽，需經審核。"
                                ) : (
                                    <>
                                        需先
                                        <Link
                                            href="/account/verification"
                                            className="mx-1 text-ink underline underline-offset-4"
                                        >
                                            完成身分驗證
                                        </Link>
                                        才能交換。
                                    </>
                                )
                            },
                            {
                                value: "public",
                                label: "公開備審",
                                description: "所有人都能瀏覽與下載，需經審核。"
                            }
                        ]}
                    />
                    {reviewed && (
                        <p className="font-sans text-sm text-copy-muted">
                            已分享的備審修改後需要重新送審，審核通過前其他人看不到這份備審。
                        </p>
                    )}
                </FieldGroup>
            </div>

            <section aria-labelledby="portfolio-items" className="flex flex-col gap-4">
                <div className="flex flex-wrap items-baseline justify-between gap-2 border-t border-ink/10 pt-6">
                    <h2 id="portfolio-items" className="font-sans text-xl font-medium text-ink">
                        檔案
                    </h2>
                    <p className="font-sans text-sm text-copy-muted">
                        每個項目上傳一個 PDF，單檔上限 {maxSizeLabel}。
                    </p>
                </div>
                <ol className="flex flex-col gap-4">
                    {items.map((item, index) => (
                        <ItemFields
                            key={item.key}
                            index={index}
                            item={item}
                            errors={showErrors ? errors.items[index] : undefined}
                            removable={items.length > 1}
                            onRename={(name) => updateItem(item.key, { name })}
                            onPickFile={(file) => pickFile(item.key, file)}
                            onRemove={() =>
                                setItems((current) =>
                                    current.filter((other) => other.key !== item.key)
                                )
                            }
                        />
                    ))}
                </ol>
                {items.length < PORTFOLIO_ITEM_LIMIT && (
                    <button
                        type="button"
                        onClick={() => setItems((current) => [...current, blankItem()])}
                        className="flex h-12 cursor-pointer items-center justify-center gap-2 rounded-[var(--radius-panel)] border border-dashed border-ink/25 font-sans text-base text-ink focus-ring transition-colors hover:border-ink/45 hover:bg-ink/5"
                    >
                        <Plus aria-hidden className="size-5" />
                        新增一個項目
                    </button>
                )}
            </section>

            <div className="flex flex-col gap-3 border-t border-ink/10 pt-6">
                <ErrorText>{error}</ErrorText>
                {showErrors && !valid && (
                    <ErrorText>還有欄位沒有填寫完成，請檢查上方標示的項目。</ErrorText>
                )}
                <div className="flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
                    <Button asChild variant="secondary" size="base">
                        <Link href={OVERVIEW_HREF}>取消</Link>
                    </Button>
                    {shared ? (
                        <>
                            <Button type="submit" variant="secondary" size="base" disabled={saving}>
                                儲存草稿
                            </Button>
                            <SubmitReviewDialog
                                trigger={
                                    <Button
                                        type="button"
                                        size="base"
                                        disabled={saving}
                                        onClick={(event) => {
                                            // Point out missing fields instead of
                                            // opening the checklist.
                                            if (!valid) {
                                                event.preventDefault();
                                                setShowErrors(true);
                                            }
                                        }}
                                    >
                                        {saving ? "處理中…" : "儲存並送出審核"}
                                    </Button>
                                }
                                onConfirm={() => save(true)}
                            />
                        </>
                    ) : (
                        <Button type="submit" size="base" disabled={saving}>
                            {saving ? "儲存中…" : "儲存"}
                        </Button>
                    )}
                </div>
            </div>
        </form>
    );
}

function ItemFields({
    index,
    item,
    errors,
    removable,
    onRename,
    onPickFile,
    onRemove
}: {
    index: number;
    item: DraftItem;
    errors?: { name: string | null; file: string | null };
    removable: boolean;
    onRename: (name: string) => void;
    onPickFile: (file: File) => void;
    onRemove: () => void;
}) {
    const label = `項目 ${index + 1}`;
    const current = item.file
        ? { name: item.file.name, size: item.file.size, note: "尚未儲存" }
        : item.existing && {
              name: item.existing.file_name,
              size: item.existing.file_size_bytes,
              note: `上傳於 ${formatDateTime(item.existing.updated_at)}`
          };

    return (
        <li className="flex flex-col gap-4 rounded-[var(--radius-panel)] border border-ink/10 bg-surface p-4 sm:p-5">
            <div className="flex items-center justify-between gap-3">
                <h3 className="font-sans text-base font-medium text-ink">{label}</h3>
                {removable && (
                    <IconButton
                        type="button"
                        label={`移除${label}`}
                        icon={<Trash2 className="size-5" />}
                        className="size-10 text-copy-muted hover:bg-ink/5 hover:text-ink"
                        onClick={onRemove}
                    />
                )}
            </div>
            <Field label="項目名稱" required error={errors?.name}>
                <Input
                    value={item.name}
                    maxLength={PORTFOLIO_ITEM_NAME_MAX_LENGTH}
                    placeholder={`例如「自傳」，限 ${PORTFOLIO_ITEM_NAME_MAX_LENGTH} 字`}
                    onChange={(event) => onRename(event.target.value)}
                />
            </Field>
            <Field label="PDF 檔案" required error={errors?.file}>
                <div className="flex flex-col gap-3">
                    {current && (
                        <p className="flex min-w-0 items-center gap-2 font-sans text-sm text-copy-muted">
                            <FileText aria-hidden className="size-4 shrink-0" />
                            <span className="min-w-0 truncate text-ink">{current.name}</span>
                            <span className="shrink-0">
                                · {formatBytes(current.size)} · {current.note}
                            </span>
                        </p>
                    )}
                    <FileUpload
                        accept={PORTFOLIO_FILE_ACCEPT}
                        label={current ? "更換檔案" : "上傳 PDF"}
                        onFileSelected={onPickFile}
                    />
                </div>
            </Field>
        </li>
    );
}

/** Text input with the school's program names as datalist suggestions. */
function ProgramInput({
    value,
    onChange,
    suggestions
}: {
    value: string;
    onChange: (value: string) => void;
    suggestions: string[];
}) {
    const listId = useId();
    return (
        <>
            <Input
                list={listId}
                value={value}
                autoComplete="off"
                placeholder="例如「資訊工程學系」"
                onChange={(event) => onChange(event.target.value)}
            />
            <datalist id={listId}>
                {suggestions.map((name) => (
                    <option key={name} value={name} />
                ))}
            </datalist>
        </>
    );
}

/** Program names a school currently publishes, for suggestions only. */
function usePrograms(schoolCode?: string) {
    const [programs, setPrograms] = useState<{ code: string; names: string[] } | null>(null);
    useEffect(() => {
        if (!schoolCode) return;
        const controller = new AbortController();
        listAdmissionPrograms({ schoolCode, limit: 200, signal: controller.signal }).then(
            ({ data }) =>
                setPrograms({
                    code: schoolCode,
                    names: Array.from(new Set(data.map((item) => item.admission_program_name)))
                }),
            // Suggestions are optional; free text still works.
            () => undefined
        );
        return () => controller.abort();
    }, [schoolCode]);
    return useMemo(
        () => (programs && programs.code === schoolCode ? programs.names : []),
        [programs, schoolCode]
    );
}
