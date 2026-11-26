"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Plus } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import Button from "../../../components/button";
import ExperienceCard from "../../../components/experience-card";
import Field, { FieldGroup } from "../../../components/field";
import FileUpload from "../../../components/file-upload";
import { Input, Textarea } from "../../../components/input";
import RadioGroup from "../../../components/radio-group";
import Select from "../../../components/select";
import { ErrorText, Resource, StatusText } from "../../../components/status";
import { ToggleChipGroup } from "../../../components/toggle-chip";
import {
    createExperience,
    createExperienceRevision,
    EXPERIENCE_SUMMARY_MAX_LENGTH,
    EXPERIENCE_TAG_LIMIT,
    EXPERIENCE_TITLE_MAX_LENGTH,
    firstBodyImage,
    getMyExperience,
    listExperienceTags,
    normalizeTags,
    submitExperienceRevision,
    uploadExperienceThumbnail,
    type ExperienceThumbnail,
    type MyExperience
} from "../../../lib/api/content";
import {
    ARTICLE_IMPORT_ACCEPT,
    ARTICLE_IMPORT_MAX_BYTES,
    importArticleFile
} from "../../../lib/article-import";
import { describeError } from "../../../lib/describe-error";
import { formatBytes } from "../../../lib/format";
import { useAction, useResource } from "../../../lib/use-resource";
import AccountPage from "../../account-page";
import { articlesNoticeHref } from "../articles-notice";

const SUGGESTION_COUNT = 12;
const THUMBNAIL_MAX_BYTES = 5 * 1024 * 1024;

/** The backend stores an empty field as "-". */
const blank = (value?: string) => (value && value !== "-" ? value : "");

// The backend stores both as free text; these keep entries consistent.
const authorTypes = ["特選錄取生", "特選應考生", "家長", "師長", "其他"];
const admissionOutcomes = ["正取", "備取", "備取遞補", "未錄取", "尚未放榜", "不適用"];

/** Options with the stored value kept even if it predates the list. */
function optionsWith(values: string[], current: string) {
    const all = current && !values.includes(current) ? [current, ...values] : values;
    return all.map((value) => ({ value, label: value }));
}

export default function ExperienceEditor() {
    const id = useSearchParams().get("id")?.trim() ?? "";
    const load = useCallback(
        (signal: AbortSignal) =>
            id ? getMyExperience(id, { signal }).then(({ data }) => data) : Promise.resolve(null),
        [id]
    );
    const { state, reload } = useResource(load);

    return (
        <AccountPage
            title={id ? "編輯文章" : "新增文章"}
            description="分享你的特選準備心得。文章經管理員審核通過後才會公開。"
        >
            <Resource state={state} reload={reload}>
                {(experience) => (
                    <EditorForm
                        key={experience?.current_revision_id ?? "new"}
                        experience={experience}
                    />
                )}
            </Resource>
        </AccountPage>
    );
}

function EditorForm({ experience }: { experience: MyExperience | null }) {
    const router = useRouter();
    const [title, setTitle] = useState(experience?.title ?? "");
    const [summary, setSummary] = useState(experience?.summary ?? "");
    const [body, setBody] = useState(experience?.body ?? "");
    const [authorType, setAuthorType] = useState(blank(experience?.author_type));
    const [outcome, setOutcome] = useState(blank(experience?.admission_outcome));
    const [thumbnail, setThumbnail] = useState<ExperienceThumbnail>(
        experience?.thumbnail ?? { source: "body" }
    );
    const [tags, setTags] = useState<string[]>(experience?.tags ?? []);
    const [showErrors, setShowErrors] = useState(false);
    const { pending, error, run } = useAction();
    const saving = pending !== null;

    const errors = {
        title: title.trim() ? null : "請輸入標題。",
        body: body.trim() ? null : "請輸入內文，或從檔案匯入。",
        tags: tags.length > 0 ? null : "請至少加入一個關鍵字。",
        thumbnail:
            thumbnail.source === "upload" && !thumbnail.url ? "請上傳縮圖，或改選其他選項。" : null
    };
    const valid = Object.values(errors).every((message) => !message);
    const shown = (message: string | null) => (showErrors ? message : null);

    function save(submit: boolean) {
        setShowErrors(true);
        if (!valid) return;
        const input = {
            title: title.trim(),
            body,
            author_type: authorType,
            admission_outcome: outcome,
            summary,
            thumbnail,
            tags
        };
        void run(async () => {
            const { data: draft } = experience
                ? await createExperienceRevision(experience.id, input)
                : await createExperience(input);
            if (submit) await submitExperienceRevision(draft.current_revision_id);
            router.push(articlesNoticeHref(submit ? "submitted" : "saved"));
        });
    }

    return (
        <form
            noValidate
            className="flex flex-col gap-8"
            onSubmit={(event) => {
                event.preventDefault();
                save(false);
            }}
        >
            <ImportPanel
                onImport={(imported) => {
                    if (imported.title) {
                        setTitle(imported.title.slice(0, EXPERIENCE_TITLE_MAX_LENGTH));
                    }
                    setBody(imported.body);
                    if (imported.tags.length) setTags(normalizeTags([...tags, ...imported.tags]));
                }}
                hasContent={Boolean(title.trim() || body.trim())}
            />

            <div className="flex flex-col gap-6 border-t border-ink/10 pt-8">
                <Field
                    label="標題"
                    required
                    hint={`${Array.from(title).length} / ${EXPERIENCE_TITLE_MAX_LENGTH}`}
                    error={shown(errors.title)}
                >
                    <Input
                        value={title}
                        maxLength={EXPERIENCE_TITLE_MAX_LENGTH}
                        placeholder="從檔案匯入時會自動帶入"
                        onChange={(event) => setTitle(event.target.value)}
                    />
                </Field>
                <Field label="摘要" hint="顯示在文章卡片上；留空時使用內文開頭。">
                    <Textarea
                        value={summary}
                        rows={3}
                        maxLength={EXPERIENCE_SUMMARY_MAX_LENGTH}
                        placeholder={`用幾句話介紹這篇文章，上限 ${EXPERIENCE_SUMMARY_MAX_LENGTH} 字`}
                        onChange={(event) => setSummary(event.target.value)}
                        className="min-h-24"
                    />
                </Field>
                <Field
                    label="內文"
                    required
                    hint="支援 Markdown：# 標題、**粗體**、- 清單、[連結](https://…)、![圖片](https://…)。"
                    error={shown(errors.body)}
                >
                    <Textarea
                        value={body}
                        rows={14}
                        onChange={(event) => setBody(event.target.value)}
                        className="min-h-80 font-mono text-sm sm:text-base"
                    />
                </Field>
                <div className="grid gap-5 sm:grid-cols-2">
                    <Field label="作者身分">
                        <Select
                            value={authorType}
                            placeholder="不指定"
                            options={optionsWith(authorTypes, authorType)}
                            onChange={(event) => setAuthorType(event.target.value)}
                        />
                    </Field>
                    <Field label="錄取結果">
                        <Select
                            value={outcome}
                            placeholder="不指定"
                            options={optionsWith(admissionOutcomes, outcome)}
                            onChange={(event) => setOutcome(event.target.value)}
                        />
                    </Field>
                </div>
                <ThumbnailField
                    value={thumbnail}
                    onChange={setThumbnail}
                    bodyImage={firstBodyImage(body)}
                    error={shown(errors.thumbnail)}
                />
                <TagsField tags={tags} onChange={setTags} error={shown(errors.tags)} />
            </div>

            <section
                aria-labelledby="article-preview"
                className="flex flex-col gap-4 border-t border-ink/10 pt-8"
            >
                <h2 id="article-preview" className="font-sans text-xl font-medium text-ink">
                    卡片預覽
                </h2>
                <ExperienceCard
                    href={null}
                    experience={{
                        id: experience?.id ?? "preview",
                        title: title.trim(),
                        body,
                        summary,
                        thumbnail,
                        tags,
                        updated_at: experience?.updated_at ?? new Date().toISOString()
                    }}
                />
            </section>

            <div className="flex flex-col gap-3 border-t border-ink/10 pt-6">
                {experience && (
                    <p className="font-sans text-sm text-copy-muted">
                        儲存後會建立第 {experience.revision_number + 1}{" "}
                        版，已公開的版本在新版通過審核前不會改變。
                    </p>
                )}
                <ErrorText>{error}</ErrorText>
                {showErrors && !valid && (
                    <ErrorText>還有欄位沒有填寫完成，請檢查上方標示的項目。</ErrorText>
                )}
                <div className="flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
                    <Button asChild variant="secondary" size="base">
                        <Link href="/account/articles/">取消</Link>
                    </Button>
                    <Button type="submit" variant="secondary" size="base" disabled={saving}>
                        儲存草稿
                    </Button>
                    <Button type="button" size="base" disabled={saving} onClick={() => save(true)}>
                        {saving ? "處理中…" : "提交審核"}
                    </Button>
                </div>
            </div>
        </form>
    );
}

/** .md / .docx import; it replaces the title and body, and says so. */
function ImportPanel({
    onImport,
    hasContent
}: {
    onImport: (article: Awaited<ReturnType<typeof importArticleFile>>) => void;
    hasContent: boolean;
}) {
    const [status, setStatus] = useState<string | null>(null);
    const { pending, error, setError, run } = useAction();

    function pick(file: File) {
        setStatus(null);
        if (file.size > ARTICLE_IMPORT_MAX_BYTES) {
            setError(`檔案超過 ${formatBytes(ARTICLE_IMPORT_MAX_BYTES)}，請確認是否選錯檔案。`);
            return;
        }
        void run(file.name, async () => {
            const article = await importArticleFile(file).catch((cause: unknown) => {
                throw new Error(`無法讀取 ${file.name}：${describeError(cause)}`);
            });
            if (!article.body) throw new Error(`${file.name} 裡沒有可匯入的內容。`);
            onImport(article);
            setStatus(
                hasContent
                    ? `已從 ${file.name} 匯入，原本的標題與內文已被取代。`
                    : `已從 ${file.name} 匯入，請確認內容後再送出。`
            );
        });
    }

    const rows = [
        {
            title: "上傳 .md 檔案",
            hint: "用 HackMD 撰寫？在選單選擇「下載 → Markdown」即可取得 .md 檔。",
            accept: ARTICLE_IMPORT_ACCEPT.markdown
        },
        {
            title: "上傳 .docx 檔案",
            hint: "用 Word 或 Google Docs 撰寫？另存或下載為 .docx 即可。",
            accept: ARTICLE_IMPORT_ACCEPT.docx
        }
    ];

    return (
        <section aria-labelledby="article-import" className="flex flex-col gap-3">
            <h2 id="article-import" className="font-sans text-xl font-medium text-ink">
                從檔案匯入
                <span className="ml-2 font-sans text-sm font-normal text-copy-muted">選填</span>
            </h2>
            <div className="flex flex-col divide-y divide-ink/10 rounded-[var(--radius-panel)] border border-ink/10 bg-surface">
                {rows.map((row) => (
                    <div
                        key={row.title}
                        className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between sm:px-6"
                    >
                        <div className="min-w-0">
                            <p className="font-sans text-base font-medium text-ink">{row.title}</p>
                            <p className="font-sans text-sm text-copy-muted">{row.hint}</p>
                        </div>
                        <FileUpload
                            accept={row.accept}
                            label="選擇檔案"
                            disabled={pending !== null}
                            onFileSelected={pick}
                        />
                    </div>
                ))}
            </div>
            <StatusText tone="success">{pending ? "正在讀取檔案…" : status}</StatusText>
            <ErrorText>{error}</ErrorText>
        </section>
    );
}

function ThumbnailField({
    value,
    onChange,
    bodyImage,
    error
}: {
    value: ExperienceThumbnail;
    onChange: (value: ExperienceThumbnail) => void;
    bodyImage?: string;
    error: string | null;
}) {
    const { pending, error: uploadError, setError, run } = useAction();
    const uploadedUrl = value.source === "upload" ? value.url : "";

    function upload(file: File) {
        if (!file.type.startsWith("image/")) return setError("請選擇圖片檔。");
        if (file.size > THUMBNAIL_MAX_BYTES) {
            return setError(`圖片超過 ${formatBytes(THUMBNAIL_MAX_BYTES)} 上限。`);
        }
        void run(async () => {
            const { data } = await uploadExperienceThumbnail(file);
            onChange({ source: "upload", url: data.url });
        });
    }

    return (
        <FieldGroup label="縮圖" error={error ?? uploadError}>
            <RadioGroup
                value={value.source}
                onValueChange={(source) =>
                    onChange(
                        source === "upload"
                            ? { source: "upload", url: uploadedUrl }
                            : { source: source as "body" | "none" }
                    )
                }
                options={[
                    {
                        value: "body",
                        label: "使用文章內的第一張圖片",
                        description: bodyImage ? undefined : "目前內文沒有圖片，會先顯示預設圖樣。"
                    },
                    { value: "upload", label: "自行上傳圖片" },
                    { value: "none", label: "不使用縮圖", description: "以預設圖樣代替。" }
                ]}
            />
            {value.source === "upload" && (
                <div className="flex flex-col gap-3 pl-8">
                    {value.url && (
                        // A data/remote URL of unknown size; see ExperienceCard.
                        // eslint-disable-next-line @next/next/no-img-element
                        <img
                            src={value.url}
                            alt="目前的縮圖"
                            className="aspect-video w-full max-w-xs rounded-[var(--radius-control)] border border-ink/10 object-cover"
                        />
                    )}
                    <FileUpload
                        accept="image/png,image/jpeg,image/webp"
                        label={pending ? "上傳中…" : value.url ? "更換圖片" : "上傳圖片"}
                        disabled={pending !== null}
                        onFileSelected={upload}
                    />
                </div>
            )}
        </FieldGroup>
    );
}

function TagsField({
    tags,
    onChange,
    error
}: {
    tags: string[];
    onChange: (tags: string[]) => void;
    error: string | null;
}) {
    const [tagInput, setTagInput] = useState("");
    const [suggestions, setSuggestions] = useState<string[]>([]);

    useEffect(() => {
        // Suggestions are optional; a failure just leaves free input.
        listExperienceTags().then(
            ({ data }) => setSuggestions(data.slice(0, SUGGESTION_COUNT).map((item) => item.tag)),
            () => undefined
        );
    }, []);

    const full = tags.length >= EXPERIENCE_TAG_LIMIT;
    const chips = Array.from(new Set([...tags, ...suggestions]));

    function addTypedTag() {
        onChange(normalizeTags([...tags, tagInput]));
        setTagInput("");
    }

    return (
        <FieldGroup
            label="關鍵字"
            required
            note={`${tags.length} / ${EXPERIENCE_TAG_LIMIT}`}
            hint="至少一個，最多八個。讀者可以用關鍵字找到你的文章。"
            error={error}
        >
            {chips.length > 0 && (
                <ToggleChipGroup
                    options={chips.map((tag) => ({
                        value: tag,
                        label: `#${tag}`,
                        disabled: full && !tags.includes(tag)
                    }))}
                    value={tags}
                    onValueChange={(next) => onChange(normalizeTags(next))}
                />
            )}
            <div className="flex gap-2">
                <Input
                    value={tagInput}
                    aria-label="新增關鍵字"
                    placeholder={full ? "已達關鍵字上限" : "輸入關鍵字後按 Enter"}
                    disabled={full}
                    onChange={(event) => setTagInput(event.target.value)}
                    onKeyDown={(event) => {
                        if (event.key === "Enter" && !event.nativeEvent.isComposing) {
                            event.preventDefault();
                            if (tagInput.trim()) addTypedTag();
                        }
                    }}
                />
                <Button
                    type="button"
                    variant="secondary"
                    size="base"
                    className="shrink-0 gap-1 px-4"
                    disabled={full || !tagInput.trim()}
                    onClick={addTypedTag}
                >
                    <Plus aria-hidden className="size-4" />
                    新增
                </Button>
            </div>
        </FieldGroup>
    );
}
