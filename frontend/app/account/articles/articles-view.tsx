"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { ExternalLink, Pencil, Plus } from "lucide-react";
import { useState } from "react";
import Button from "../../components/button";
import ConfirmDialog from "../../components/confirm-dialog";
import ExperienceCard from "../../components/experience-card";
import {
    EmptyState,
    ErrorText,
    Resource,
    StatusBadge,
    StatusText,
    type BadgeTone
} from "../../components/status";
import {
    listMyExperiences,
    submitExperienceRevision,
    unpublishExperience,
    type ExperienceReviewStatus,
    type MyExperience
} from "../../lib/api/content";
import { experienceEditorHref, experienceHref } from "../../lib/routes";
import { useAction, useResource } from "../../lib/use-resource";
import AccountPage from "../account-page";
import { articlesNoticeText } from "./articles-notice";

const loadMine = (signal: AbortSignal) => listMyExperiences({ signal }).then(({ data }) => data);

const reviewBadge: Record<ExperienceReviewStatus, { label: string; tone: BadgeTone }> = {
    draft: { label: "草稿", tone: "neutral" },
    pending_review: { label: "審核中", tone: "pending" },
    approved: { label: "已通過審核", tone: "success" },
    rejected: { label: "未通過審核", tone: "danger" }
};

type SectionKey = "pending" | "drafts" | "published" | "unpublished";

const sections: { key: SectionKey; title: string }[] = [
    { key: "pending", title: "待審核的文章" },
    { key: "drafts", title: "草稿" },
    { key: "published", title: "已發布的文章" },
    { key: "unpublished", title: "已下架的文章" }
];

/** The latest revision's review decides where an article is listed; once
 * approved, its visibility does. */
function sectionOf(experience: MyExperience): SectionKey {
    if (experience.review_status === "pending_review") return "pending";
    if (experience.review_status !== "approved") return "drafts";
    return experience.visibility === "published" ? "published" : "unpublished";
}

export default function ArticlesView() {
    const notice = articlesNoticeText(useSearchParams().get("notice"));
    const { state, reload, setData } = useResource(loadMine);
    const { pending, error, run } = useAction();
    const [actionNotice, setActionNotice] = useState<string | null>(null);

    const patch = (id: string, update: Partial<MyExperience>) =>
        setData((items) => items.map((item) => (item.id === id ? { ...item, ...update } : item)));

    async function act(id: string, action: () => Promise<void>, success: string) {
        setActionNotice(null);
        if (await run(id, action)) setActionNotice(success);
    }

    return (
        <AccountPage title="文章發佈" description="分享你的特選準備心得，審核通過後就會公開。">
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-[var(--radius-panel)] border border-ink/10 bg-surface p-4 sm:px-6">
                <p className="font-sans text-base text-ink">新增一篇文章</p>
                <Button asChild size="sm" className="gap-2">
                    <Link href={experienceEditorHref()}>
                        <Plus aria-hidden className="size-4" />
                        新增文章
                    </Link>
                </Button>
            </div>

            <ErrorText>{error}</ErrorText>
            <StatusText>{actionNotice ?? notice}</StatusText>

            <Resource state={state} reload={reload}>
                {(experiences) =>
                    experiences.length === 0 ? (
                        <EmptyState
                            title="還沒有發佈過文章"
                            description="寫下你的申請經驗，幫助下一屆的特選生。"
                        />
                    ) : (
                        sections.map((section) => {
                            const items = experiences.filter(
                                (item) => sectionOf(item) === section.key
                            );
                            if (items.length === 0) return null;
                            return (
                                <section
                                    key={section.key}
                                    aria-labelledby={`articles-${section.key}`}
                                    className="flex flex-col gap-4 border-t border-ink/10 pt-6"
                                >
                                    <h2
                                        id={`articles-${section.key}`}
                                        className="font-sans text-2xl font-medium text-ink"
                                    >
                                        {section.title}
                                    </h2>
                                    <ul className="flex flex-col gap-5">
                                        {items.map((experience) => (
                                            <li key={experience.id}>
                                                <ExperienceItem
                                                    experience={experience}
                                                    busy={pending === experience.id}
                                                    onSubmit={() =>
                                                        act(
                                                            experience.id,
                                                            async () => {
                                                                const { data } =
                                                                    await submitExperienceRevision(
                                                                        experience.current_revision_id
                                                                    );
                                                                patch(experience.id, {
                                                                    ...data,
                                                                    review_status: "pending_review"
                                                                });
                                                            },
                                                            "已送出審核，通過後會公開。"
                                                        )
                                                    }
                                                    onUnpublish={() =>
                                                        act(
                                                            experience.id,
                                                            async () => {
                                                                await unpublishExperience(
                                                                    experience.id
                                                                );
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
                                </section>
                            );
                        })
                    )
                }
            </Resource>
        </AccountPage>
    );
}

function ExperienceItem({
    experience,
    busy,
    onSubmit,
    onUnpublish
}: {
    experience: MyExperience;
    busy: boolean;
    onSubmit: () => void;
    onUnpublish: () => void;
}) {
    const review = reviewBadge[experience.review_status];
    const published = experience.visibility === "published";
    const canSubmit =
        experience.review_status === "draft" || experience.review_status === "rejected";

    return (
        <article aria-label={experience.title} className="flex flex-col gap-3">
            <ExperienceCard
                experience={experience}
                href={published ? experienceHref(experience.id) : null}
                eyebrow={
                    <span className="flex flex-wrap items-center gap-2">
                        <StatusBadge tone={review.tone}>
                            第 {experience.revision_number} 版・{review.label}
                        </StatusBadge>
                        {published && experience.review_status !== "approved" && (
                            <StatusBadge tone="success">目前公開中</StatusBadge>
                        )}
                    </span>
                }
            />
            <div className="flex flex-wrap gap-2">
                <Button asChild variant="secondary" size="sm" className="gap-2">
                    <Link href={experienceEditorHref(experience.id)}>
                        <Pencil aria-hidden className="size-4" />
                        編輯
                    </Link>
                </Button>
                {canSubmit && (
                    <Button size="sm" disabled={busy} onClick={onSubmit}>
                        送出審核
                    </Button>
                )}
                {published && (
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
        </article>
    );
}
