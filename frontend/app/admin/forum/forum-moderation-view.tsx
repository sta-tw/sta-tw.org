"use client";

import { useEffect, useState } from "react";
import { Archive, ChevronDown, ChevronUp, Lock, Trash2, Unlock } from "lucide-react";
import {
    adminListForumPosts,
    adminListForumThreads,
    archiveForumPost,
    archiveForumThread,
    deleteForumPost,
    deleteForumThread,
    lockForumThread,
    unlockForumThread,
    type AdminForumPost,
    type AdminForumThread,
    type ForumPostStatus,
    type ForumThreadStatus
} from "../../lib/api/admin";
import { useAdmin } from "../admin-context";
import {
    AdminPanel,
    Badge,
    type BadgeTone,
    ConfirmDialog,
    describeError,
    EmptyState,
    ErrorText,
    LoadingState,
    panelClassName
} from "../admin-ui";

const threadStatusLabel: Record<ForumThreadStatus, string> = {
    published: "公開中",
    locked: "已關閉回覆",
    hidden: "已隱藏",
    removed: "已刪除",
    archived: "已封存（證據保留）"
};

const threadStatusTone: Record<ForumThreadStatus, BadgeTone> = {
    published: "positive",
    locked: "warning",
    hidden: "neutral",
    removed: "negative",
    archived: "warning"
};

const postStatusLabel: Record<ForumPostStatus, string> = {
    published: "公開中",
    hidden: "已隱藏",
    removed: "已刪除",
    archived: "已封存（證據保留）"
};

function formatDate(iso: string): string {
    try {
        return new Date(iso).toLocaleString("zh-TW", { dateStyle: "medium", timeStyle: "short" });
    } catch {
        return iso;
    }
}

function PostRow({
    post,
    mfaCode,
    onChanged
}: {
    post: AdminForumPost;
    mfaCode: string;
    onChanged: (postId: string, status: ForumPostStatus) => void;
}) {
    const [confirmOpen, setConfirmOpen] = useState(false);
    const [archiveConfirmOpen, setArchiveConfirmOpen] = useState(false);
    const [pending, setPending] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const actionable = post.status === "published";

    async function confirmDelete() {
        setPending(true);
        setError(null);
        try {
            await deleteForumPost(post.id, mfaCode || undefined);
            setConfirmOpen(false);
            onChanged(post.id, "removed");
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setPending(false);
        }
    }

    async function confirmArchive() {
        setPending(true);
        setError(null);
        try {
            await archiveForumPost(post.id, mfaCode || undefined);
            setArchiveConfirmOpen(false);
            onChanged(post.id, "archived");
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setPending(false);
        }
    }

    return (
        <div className="rounded-[var(--radius-small)] border border-ink/10 bg-surface p-3 text-sm">
            <div className="flex flex-wrap items-baseline justify-between gap-2">
                <span className="font-medium text-ink">{post.author_username}</span>
                <span className="flex items-center gap-2 text-xs text-ink/50">
                    {formatDate(post.created_at)}
                    {post.status !== "published" && (
                        <Badge tone={post.status === "archived" ? "warning" : "negative"}>
                            {postStatusLabel[post.status]}
                        </Badge>
                    )}
                </span>
            </div>
            <p className="mt-1 whitespace-pre-wrap text-ink/80">{post.body}</p>
            {error && <ErrorText>{error}</ErrorText>}
            {actionable && (
                <div className="mt-2 flex items-center gap-4">
                    <button
                        type="button"
                        onClick={() => setArchiveConfirmOpen(true)}
                        className="inline-flex items-center gap-1 font-sans text-xs text-ink/60 hover:text-ink hover:underline"
                    >
                        <Archive className="h-3.5 w-3.5" aria-hidden />
                        封存做為證據
                    </button>
                    <button
                        type="button"
                        onClick={() => setConfirmOpen(true)}
                        className="inline-flex items-center gap-1 font-sans text-xs text-red-600 hover:underline"
                    >
                        <Trash2 className="h-3.5 w-3.5" aria-hidden />
                        刪除這則回覆
                    </button>
                </div>
            )}
            <ConfirmDialog
                open={confirmOpen}
                onOpenChange={setConfirmOpen}
                title="刪除這則回覆？"
                description={`「${post.author_username}」的這則回覆將從論壇上移除，使用者將看不到內容。`}
                confirmLabel="刪除"
                pending={pending}
                onConfirm={() => void confirmDelete()}
            />
            <ConfirmDialog
                open={archiveConfirmOpen}
                onOpenChange={setArchiveConfirmOpen}
                title="封存這則回覆？"
                description="內容會從公開論壇移除並保留做為證據，僅管理員看得到，之後無法透過介面復原為公開狀態。"
                confirmLabel="封存"
                pending={pending}
                onConfirm={() => void confirmArchive()}
            />
        </div>
    );
}

function ThreadRow({
    thread,
    mfaCode,
    onChanged
}: {
    thread: AdminForumThread;
    mfaCode: string;
    onChanged: () => void;
}) {
    const [expanded, setExpanded] = useState(false);
    const [posts, setPosts] = useState<AdminForumPost[] | null>(null);
    const [postsError, setPostsError] = useState<string | null>(null);
    const [postsLoading, setPostsLoading] = useState(false);
    const [actionPending, setActionPending] = useState(false);
    const [actionError, setActionError] = useState<string | null>(null);
    const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);
    const [archiveConfirmOpen, setArchiveConfirmOpen] = useState(false);

    async function toggle() {
        if (expanded) {
            setExpanded(false);
            return;
        }
        setExpanded(true);
        if (posts) return;
        setPostsLoading(true);
        setPostsError(null);
        try {
            const response = await adminListForumPosts(thread.id, mfaCode || undefined);
            setPosts(response.data);
        } catch (cause) {
            setPostsError(describeError(cause));
        } finally {
            setPostsLoading(false);
        }
    }

    async function toggleLock() {
        setActionPending(true);
        setActionError(null);
        try {
            if (thread.status === "locked") {
                await unlockForumThread(thread.id, mfaCode || undefined);
            } else {
                await lockForumThread(thread.id, mfaCode || undefined);
            }
            onChanged();
        } catch (cause) {
            setActionError(describeError(cause));
        } finally {
            setActionPending(false);
        }
    }

    async function confirmDeleteThread() {
        setActionPending(true);
        setActionError(null);
        try {
            await deleteForumThread(thread.id, mfaCode || undefined);
            setDeleteConfirmOpen(false);
            onChanged();
        } catch (cause) {
            setActionError(describeError(cause));
        } finally {
            setActionPending(false);
        }
    }

    async function confirmArchiveThread() {
        setActionPending(true);
        setActionError(null);
        try {
            await archiveForumThread(thread.id, mfaCode || undefined);
            setArchiveConfirmOpen(false);
            onChanged();
        } catch (cause) {
            setActionError(describeError(cause));
        } finally {
            setActionPending(false);
        }
    }

    const removed = thread.status === "removed";
    const archived = thread.status === "archived";
    const locked = thread.status === "locked";

    return (
        <AdminPanel className="p-0">
            <div className="flex flex-wrap items-center justify-between gap-3 p-4">
                <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                        <p className="font-serif text-lg text-ink">{thread.title}</p>
                        <Badge tone={threadStatusTone[thread.status]}>{threadStatusLabel[thread.status]}</Badge>
                    </div>
                    <p className="mt-1 font-sans text-xs text-ink/50">
                        發文者：{thread.author_username} ・ {formatDate(thread.created_at)}
                    </p>
                </div>
                <div className="flex shrink-0 items-center gap-3">
                    <button
                        type="button"
                        onClick={toggle}
                        className="inline-flex items-center gap-1 font-sans text-sm text-ink/70 hover:text-ink"
                    >
                        查看內容
                        {expanded ? <ChevronUp className="h-4 w-4" aria-hidden /> : <ChevronDown className="h-4 w-4" aria-hidden />}
                    </button>
                    {!removed && !archived && (
                        <button
                            type="button"
                            disabled={actionPending}
                            onClick={() => void toggleLock()}
                            className="inline-flex items-center gap-1 font-sans text-sm text-ink/70 hover:text-ink disabled:opacity-50"
                        >
                            {locked ? (
                                <>
                                    <Unlock className="h-4 w-4" aria-hidden />
                                    重新開放回覆
                                </>
                            ) : (
                                <>
                                    <Lock className="h-4 w-4" aria-hidden />
                                    關閉回覆
                                </>
                            )}
                        </button>
                    )}
                    {!removed && !archived && (
                        <button
                            type="button"
                            onClick={() => setArchiveConfirmOpen(true)}
                            className="inline-flex items-center gap-1 font-sans text-sm text-ink/60 hover:text-ink hover:underline"
                        >
                            <Archive className="h-4 w-4" aria-hidden />
                            封存做為證據
                        </button>
                    )}
                    {!removed && !archived && (
                        <button
                            type="button"
                            onClick={() => setDeleteConfirmOpen(true)}
                            className="inline-flex items-center gap-1 font-sans text-sm text-red-600 hover:underline"
                        >
                            <Trash2 className="h-4 w-4" aria-hidden />
                            刪除討論串
                        </button>
                    )}
                </div>
            </div>

            {actionError && (
                <div className="px-4 pb-3">
                    <ErrorText>{actionError}</ErrorText>
                </div>
            )}

            {expanded && (
                <div className="border-t border-ink/10 p-4">
                    {postsLoading ? (
                        <LoadingState />
                    ) : postsError ? (
                        <ErrorText>{postsError}</ErrorText>
                    ) : (posts ?? []).length === 0 ? (
                        <EmptyState label="這則討論串還沒有回覆" />
                    ) : (
                        <div className="flex flex-col gap-2">
                            {(posts ?? []).map((post) => (
                                <PostRow
                                    key={post.id}
                                    post={post}
                                    mfaCode={mfaCode}
                                    onChanged={(postId, status) =>
                                        setPosts((prev) =>
                                            prev ? prev.map((p) => (p.id === postId ? { ...p, status } : p)) : prev
                                        )
                                    }
                                />
                            ))}
                        </div>
                    )}
                </div>
            )}

            <ConfirmDialog
                open={deleteConfirmOpen}
                onOpenChange={setDeleteConfirmOpen}
                title="刪除這個討論串？"
                description={`「${thread.title}」整串討論（含所有回覆）將從論壇上移除，使用者將看不到內容。`}
                confirmLabel="刪除"
                pending={actionPending}
                onConfirm={() => void confirmDeleteThread()}
            />
            <ConfirmDialog
                open={archiveConfirmOpen}
                onOpenChange={setArchiveConfirmOpen}
                title="封存這個討論串？"
                description={`「${thread.title}」整串討論會從公開論壇移除並保留做為證據，僅管理員看得到，之後無法透過介面復原為公開狀態。`}
                confirmLabel="封存"
                pending={actionPending}
                onConfirm={() => void confirmArchiveThread()}
            />
        </AdminPanel>
    );
}

export default function ForumModerationView() {
    const { mfaCode } = useAdmin();
    const [threads, setThreads] = useState<AdminForumThread[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);

    async function load() {
        setLoading(true);
        setError(null);
        try {
            const response = await adminListForumThreads(undefined, mfaCode || undefined);
            setThreads(response.data);
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setLoading(false);
        }
    }

    useEffect(() => {
        load();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [mfaCode]);

    return (
        <div className="flex flex-col gap-8">
            <div>
                <h1 className="font-serif text-2xl text-ink">論壇管理</h1>
                <p className="mt-2 font-sans text-sm text-ink/60">
                    發文與回覆已限制為通過學校或畢業生身份驗證的帳號，每則討論串與回覆都記錄了發文者帳號，方便追查不當言論。「關閉回覆」會保留討論串內容但停止接受新回覆；「刪除」會把內容從公開論壇上移除；「封存」同樣會下架內容，但標記為保留做為證據（例如疑似違法情形），僅管理員看得到，且無法透過介面復原。
                </p>
            </div>

            {error && <ErrorText>{error}</ErrorText>}
            {loading && !threads ? (
                <LoadingState />
            ) : (threads ?? []).length === 0 ? (
                <div className={panelClassName}>
                    <EmptyState label="目前還沒有任何討論串" />
                </div>
            ) : (
                <div className="flex flex-col gap-3">
                    {(threads ?? []).map((thread) => (
                        <ThreadRow key={thread.id} thread={thread} mfaCode={mfaCode || ""} onChanged={load} />
                    ))}
                </div>
            )}
        </div>
    );
}
