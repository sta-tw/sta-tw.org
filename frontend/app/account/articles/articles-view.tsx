"use client";

import Link from "next/link";
import { ExternalLink, PenLine } from "lucide-react";
import { useState } from "react";
import Button from "../../components/button";
import { Card } from "../../components/card";
import ConfirmDialog from "../../components/confirm-dialog";
import HashtagBadge from "../../components/hashtag-badge";
import {
    listMyExperiences,
    submitExperienceRevision,
    unpublishExperience,
    type Experience,
    type ExperienceReviewStatus,
    type MyExperience,
    type TaggedExperience
} from "../../lib/api/content";
import AccountPage from "../account-page";
import {
    EmptyState,
    ErrorText,
    Resource,
    StatusBadge,
    StatusText,
    type BadgeTone
} from "../../components/status";
import { useAction, useResource } from "../../lib/use-resource";
import { formatDate } from "../../lib/format";
import { experienceHref } from "../../lib/routes";
import ExperienceEditor, { SUBMITTED_MESSAGE } from "./experience-editor";

const loadMine = (signal: AbortSignal) => listMyExperiences({ signal }).then(({ data }) => data);

const visibilityBadge: Record<Experience["visibility"], { label: string; tone: BadgeTone }> = {
    hidden: { label: "未公開", tone: "neutral" },
    published: { label: "已發布", tone: "success" },
    unpublished: { label: "已下架", tone: "neutral" }
};

const reviewBadge: Record<ExperienceReviewStatus, { label: string; tone: BadgeTone }> = {
    draft: { label: "草稿", tone: "neutral" },
    pending_review: { label: "審核中", tone: "pending" },
    approved: { label: "已通過審核", tone: "success" },
    rejected: { label: "未通過審核", tone: "danger" }
};

type EditorState = { experience: TaggedExperience | null } | null;

export default function ArticlesView() {
    const { state, reload, setData } = useResource(loadMine);
    const { pending, error, run } = useAction();
    const [editor, setEditor] = useState<EditorState>(null);
    const [notice, setNotice] = useState<string | null>(null);

    const patch = (id: string, update: Partial<MyExperience>) =>
        setData((items) => items.map((item) => (item.id === id ? { ...item, ...update } : item)));

    async function act(id: string, action: () => Promise<void>, success: string) {
        setNotice(null);
        if (await run(id, action)) setNotice(success);
    }

    const openEditor = (experience: TaggedExperience | null) => {
        setNotice(null);
        setEditor({ experience });
    };

    return (
        <AccountPage
            title="文章發佈"
            description="分享你的特選準備心得。文章經管理員審核通過後才會公開。"
            actions={
                !editor && (
                    <Button size="base" className="gap-2" onClick={() => openEditor(null)}>
                        <PenLine aria-hidden className="size-4" />
                        撰寫新文章
                    </Button>
                )
            }
        >
            {editor && (
                <ExperienceEditor
                    key={editor.experience?.current_revision_id ?? "new"}
                    experience={editor.experience}
                    onCancel={() => setEditor(null)}
                    onDone={(saved, text) => {
                        setEditor(null);
                        setNotice(text);
                        // Put the saved version first, like the server's
                        // most-recently-changed order.
                        setData((items) => [
                            saved,
                            ...items.filter((item) => item.id !== saved.id)
                        ]);
                    }}
                />
            )}
            <ErrorText>{error}</ErrorText>
            <StatusText>{notice}</StatusText>
            <Resource state={state} reload={reload}>
                {(experiences) =>
                    experiences.length === 0 ? (
                        !editor && (
                            <EmptyState
                                title="還沒有發佈過文章"
                                description="寫下你的申請經驗，幫助下一屆的特選生。"
                            />
                        )
                    ) : (
                        <ul className="flex flex-col gap-4">
                            {experiences.map((experience) => (
                                <li key={experience.id}>
                                    <ExperienceItem
                                        experience={experience}
                                        busy={pending === experience.id}
                                        onEdit={() => openEditor(experience)}
                                        onSubmit={() =>
                                            act(
                                                experience.id,
                                                async () => {
                                                    const { data } = await submitExperienceRevision(
                                                        experience.current_revision_id
                                                    );
                                                    patch(experience.id, {
                                                        ...data,
                                                        review_status: "pending_review"
                                                    });
                                                },
                                                SUBMITTED_MESSAGE
                                            )
                                        }
                                        onUnpublish={() =>
                                            act(
                                                experience.id,
                                                async () => {
                                                    await unpublishExperience(experience.id);
                                                    patch(experience.id, {
                                                        visibility: "unpublished"
                                                    });
                                                },
                                                "文章已下架。"
                                            )
                                        }
                                    />
                                </li>
                            ))}
                        </ul>
                    )
                }
            </Resource>
        </AccountPage>
    );
}

function ExperienceItem({
    experience,
    busy,
    onEdit,
    onSubmit,
    onUnpublish
}: {
    experience: MyExperience;
    busy: boolean;
    onEdit: () => void;
    onSubmit: () => void;
    onUnpublish: () => void;
}) {
    const visibility = visibilityBadge[experience.visibility];
    const review = reviewBadge[experience.review_status];
    const canSubmit =
        experience.review_status === "draft" || experience.review_status === "rejected";

    return (
        <Card className="flex flex-col gap-4 p-5 sm:p-6">
            <div className="flex flex-wrap items-center gap-2">
                <StatusBadge tone={visibility.tone}>{visibility.label}</StatusBadge>
                <StatusBadge tone={review.tone}>
                    第 {experience.revision_number} 版・{review.label}
                </StatusBadge>
                <time
                    dateTime={experience.updated_at}
                    className="font-sans text-sm text-copy-muted"
                >
                    更新於 {formatDate(experience.updated_at)}
                </time>
            </div>
            <h2 className="font-sans text-xl leading-snug font-medium break-words text-ink sm:text-2xl">
                {experience.title}
            </h2>
            {experience.tags.length > 0 && (
                <div className="flex flex-wrap gap-2">
                    {experience.tags.map((tag) => (
                        <HashtagBadge key={tag} tag={tag} />
                    ))}
                </div>
            )}
            <div className="flex flex-wrap gap-2">
                <Button variant="secondary" size="sm" disabled={busy} onClick={onEdit}>
                    編輯
                </Button>
                {canSubmit && (
                    <Button size="sm" disabled={busy} onClick={onSubmit}>
                        送出審核
                    </Button>
                )}
                {experience.visibility === "published" && (
                    <>
                        <Button asChild variant="secondary" size="sm" className="gap-2">
                            <Link href={experienceHref(experience.id)}>
                                <ExternalLink aria-hidden className="size-4" />
                                查看文章
                            </Link>
                        </Button>
                        <ConfirmDialog
                            trigger={
                                <Button variant="secondary" size="sm" disabled={busy}>
                                    下架
                                </Button>
                            }
                            title="下架這篇文章？"
                            description="下架後其他人將無法看到這篇文章；要重新公開需再次送出審核。"
                            confirmLabel="下架"
                            onConfirm={onUnpublish}
                        />
                    </>
                )}
            </div>
        </Card>
    );
}
