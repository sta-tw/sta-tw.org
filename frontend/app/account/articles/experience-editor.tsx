"use client";

import { Plus } from "lucide-react";
import { useEffect, useState } from "react";
import Button from "../../components/button";
import { Card } from "../../components/card";
import Field, { FieldGroup } from "../../components/field";
import { Input, Textarea } from "../../components/input";
import Select from "../../components/select";
import { ErrorText } from "../../components/status";
import { ToggleChipGroup } from "../../components/toggle-chip";
import {
    createExperience,
    createExperienceRevision,
    EXPERIENCE_TAG_LIMIT,
    listExperienceTags,
    normalizeTags,
    submitExperienceRevision,
    type MyExperience,
    type TaggedExperience
} from "../../lib/api/content";
import { useAction } from "../../lib/use-resource";

/** backend content.ValidateText: title ≤ 300 bytes. Counted in characters
 * here (CJK is 3 bytes in UTF-8), so stay well below. */
const TITLE_MAX_LENGTH = 100;
const SUGGESTION_COUNT = 12;

export const SUBMITTED_MESSAGE = "已送出審核，通過後會公開。";

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

export default function ExperienceEditor({
    experience,
    onDone,
    onCancel
}: {
    /** null starts a new draft; otherwise saving creates a new revision. */
    experience: TaggedExperience | null;
    /** Called with the saved version as "my experiences" now lists it. */
    onDone: (saved: MyExperience, message: string) => void;
    onCancel: () => void;
}) {
    const [title, setTitle] = useState(experience?.title ?? "");
    const [body, setBody] = useState(experience?.body ?? "");
    const [authorType, setAuthorType] = useState(blank(experience?.author_type));
    const [outcome, setOutcome] = useState(blank(experience?.admission_outcome));
    const [tags, setTags] = useState<string[]>(experience?.tags ?? []);
    const [tagInput, setTagInput] = useState("");
    const [suggestions, setSuggestions] = useState<string[]>([]);
    const { pending, error, run } = useAction();
    const saving = pending !== null;

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
        const next = normalizeTags([...tags, tagInput]);
        setTags(next);
        setTagInput("");
    }

    function save(submit: boolean) {
        const input = {
            title: title.trim(),
            body,
            author_type: authorType,
            admission_outcome: outcome,
            tags
        };
        void run(async () => {
            const { data: draft } = experience
                ? await createExperienceRevision(experience.id, input)
                : await createExperience(input);
            const data = submit
                ? (await submitExperienceRevision(draft.current_revision_id)).data
                : draft;
            onDone(
                {
                    ...data,
                    tags: normalizeTags(tags),
                    review_status: submit ? "pending_review" : "draft"
                },
                submit ? SUBMITTED_MESSAGE : "已儲存草稿。"
            );
        });
    }

    const canSave = title.trim() !== "" && body.trim() !== "" && !saving;

    return (
        <Card
            title={experience ? "編輯文章" : "撰寫新文章"}
            description={
                experience &&
                `儲存後會建立第 ${experience.revision_number + 1} 版，已公開的版本在新版通過審核前不會改變。`
            }
            className="flex flex-col gap-5"
        >
            <form
                className="flex flex-col gap-5"
                onSubmit={(event) => {
                    event.preventDefault();
                    save(false);
                }}
            >
                <Field label="標題" required>
                    <Input
                        value={title}
                        maxLength={TITLE_MAX_LENGTH}
                        onChange={(event) => setTitle(event.target.value)}
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
                <Field
                    label="內文"
                    required
                    hint="支援 Markdown：# 標題、**粗體**、- 清單、[連結](https://…)。"
                >
                    <Textarea
                        value={body}
                        rows={14}
                        onChange={(event) => setBody(event.target.value)}
                        className="min-h-80 font-mono text-sm sm:text-base"
                    />
                </Field>
                <FieldGroup label="標籤" note={`${tags.length} / ${EXPERIENCE_TAG_LIMIT}`}>
                    {chips.length > 0 && (
                        <ToggleChipGroup
                            options={chips.map((tag) => ({
                                value: tag,
                                label: `#${tag}`,
                                disabled: full && !tags.includes(tag)
                            }))}
                            value={tags}
                            onValueChange={(next) => setTags(normalizeTags(next))}
                        />
                    )}
                    <div className="flex gap-2">
                        <Input
                            value={tagInput}
                            aria-label="新增標籤"
                            placeholder={full ? "已達標籤上限" : "輸入新標籤後按 Enter"}
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
                <ErrorText>{error}</ErrorText>
                <div className="flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
                    <Button type="button" variant="secondary" size="base" onClick={onCancel}>
                        取消
                    </Button>
                    <Button type="submit" variant="secondary" disabled={!canSave} size="base">
                        {experience ? "儲存為新版本" : "儲存草稿"}
                    </Button>
                    <Button
                        type="button"
                        disabled={!canSave}
                        size="base"
                        onClick={() => save(true)}
                    >
                        {saving ? "處理中…" : "儲存並送出審核"}
                    </Button>
                </div>
            </form>
        </Card>
    );
}
