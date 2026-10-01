"use client";

import { useEffect, useState } from "react";
import { AlertCircle, ArrowLeft, Loader2, MessageCircle } from "lucide-react";
import { twMerge } from "tailwind-merge";
import Button from "../components/button";
import { getCurrentAccount } from "../lib/api/auth";
import {
    addPostReaction,
    createPost,
    createThread,
    listPosts,
    listSpaces,
    listThreads,
    removePostReaction,
    type ForumPost,
    type ForumSpace,
    type ForumThread
} from "../lib/api/forum";
import { ApiError, type Account } from "../lib/api/types";

type View = { name: "threads" } | { name: "posts"; thread: ForumThread };

const panelClass =
    "rounded-[var(--radius-panel)] border border-ink/10 bg-surface p-6 shadow-[var(--shadow-card)] sm:p-8";
const inputClass =
    "w-full rounded-[var(--radius-small)] border border-ink/15 bg-surface px-4 py-3 font-sans text-base text-ink outline-none transition-colors placeholder:text-copy-muted focus:border-ink/40";
const reactionOptions = ["👍", "❤️", "🎉"];

function describeError(cause: unknown): string {
    if (cause instanceof ApiError) return cause.message || `發生錯誤（${cause.code}）`;
    return "發生未知錯誤，請稍後再試。";
}

function LoadingState({ label = "載入中…" }: { label?: string }) {
    return (
        <div className="flex items-center justify-center gap-2 p-6 font-sans text-sm text-copy-muted">
            <Loader2 aria-hidden className="h-4 w-4 animate-spin" />
            {label}
        </div>
    );
}

function EmptyState({ label }: { label: string }) {
    return <p className="p-6 text-center font-sans text-sm text-copy-muted">{label}</p>;
}

function ErrorText({ children }: { children: React.ReactNode }) {
    return (
        <p className="flex items-start gap-1.5 font-sans text-sm text-red-600">
            <AlertCircle aria-hidden className="mt-0.5 h-4 w-4 shrink-0" />
            {children}
        </p>
    );
}

export default function ForumView() {
    const [account, setAccount] = useState<Account | null>(null);
    const [space, setSpace] = useState<ForumSpace | null>(null);
    const [spaceError, setSpaceError] = useState<string | null>(null);
    const [view, setView] = useState<View>({ name: "threads" });

    useEffect(() => {
        let ignore = false;
        getCurrentAccount()
            .then(({ account }) => !ignore && setAccount(account))
            .catch(() => !ignore && setAccount(null));
        return () => {
            ignore = true;
        };
    }, []);

    useEffect(() => {
        let ignore = false;
        listSpaces()
            .then(({ data }) => {
                if (ignore) return;
                // Exactly one `global` space is guaranteed to exist (seeded by
                // migration 000013); that's the only entry point this page
                // exposes today, so jump straight into it instead of making
                // people pick from a list of one.
                const globalSpace = data.find((candidate) => candidate.space_type === "global") ?? null;
                setSpace(globalSpace);
                if (!globalSpace) setSpaceError("目前沒有可用的討論空間。");
            })
            .catch((cause) => !ignore && setSpaceError(describeError(cause)));
        return () => {
            ignore = true;
        };
    }, []);

    if (spaceError) {
        return (
            <div className={panelClass}>
                <ErrorText>{spaceError}</ErrorText>
            </div>
        );
    }
    if (!space) {
        return <LoadingState label="討論區載入中…" />;
    }
    if (view.name === "posts") {
        return (
            <PostList
                account={account}
                thread={view.thread}
                onBack={() => setView({ name: "threads" })}
            />
        );
    }
    return (
        <ThreadList
            account={account}
            space={space}
            onOpenThread={(thread) => setView({ name: "posts", thread })}
        />
    );
}

function ThreadList({
    account,
    space,
    onOpenThread
}: {
    account: Account | null;
    space: ForumSpace;
    onOpenThread: (thread: ForumThread) => void;
}) {
    const [threads, setThreads] = useState<ForumThread[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [showForm, setShowForm] = useState(false);
    const [form, setForm] = useState({ title: "", body: "" });
    const [submitting, setSubmitting] = useState(false);

    useEffect(() => {
        let ignore = false;
        listThreads(space.id)
            .then(({ data }) => !ignore && setThreads(data))
            .catch((cause) => !ignore && setError(describeError(cause)));
        return () => {
            ignore = true;
        };
    }, [space.id]);

    async function handleCreate(event: React.FormEvent) {
        event.preventDefault();
        setSubmitting(true);
        setError(null);
        try {
            const { thread } = await createThread(space.id, form);
            setThreads((prev) => (prev ? [thread, ...prev] : [thread]));
            setForm({ title: "", body: "" });
            setShowForm(false);
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setSubmitting(false);
        }
    }

    const canPost = account !== null && account.identity_status !== "temporary";

    return (
        <div className="flex flex-col gap-6">
            <div className="flex flex-wrap items-start justify-between gap-4">
                <div>
                    <h1 className="font-serif text-hero-subtitle text-ink">討論區</h1>
                    <p className="mt-2 font-sans text-copy-muted">
                        全站公開的討論空間，所有人都能瀏覽；需要通過學校信箱或畢業生身份驗證才能發文與回覆。
                    </p>
                </div>
                {canPost ? (
                    <Button className="h-11 px-5 text-base" onClick={() => setShowForm((v) => !v)}>
                        {showForm ? "取消" : "發起討論"}
                    </Button>
                ) : null}
            </div>

            {account && !canPost ? (
                <p className="font-sans text-sm text-copy-muted">
                    你的帳號尚未完成身份驗證，暫時無法發文或回覆，僅能瀏覽。
                </p>
            ) : null}

            {error ? <ErrorText>{error}</ErrorText> : null}

            {showForm ? (
                <form className={twMerge(panelClass, "flex flex-col gap-4")} onSubmit={handleCreate}>
                    <input
                        className={inputClass}
                        placeholder="標題"
                        required
                        maxLength={300}
                        value={form.title}
                        onChange={(e) => setForm((f) => ({ ...f, title: e.target.value }))}
                    />
                    <textarea
                        className={twMerge(inputClass, "min-h-32 resize-y")}
                        placeholder="內容"
                        required
                        value={form.body}
                        onChange={(e) => setForm((f) => ({ ...f, body: e.target.value }))}
                    />
                    <Button type="submit" disabled={submitting} className="w-fit px-6">
                        {submitting ? "送出中…" : "送出討論"}
                    </Button>
                </form>
            ) : null}

            {threads === null ? (
                <LoadingState />
            ) : threads.length === 0 ? (
                <EmptyState label="這裡還沒有討論串，當第一個發文的人吧！" />
            ) : (
                <div className="flex flex-col gap-3">
                    {threads.map((thread) => (
                        <button
                            key={thread.id}
                            type="button"
                            onClick={() => onOpenThread(thread)}
                            className="flex items-center justify-between gap-4 rounded-[var(--radius-small)] border border-ink/10 bg-surface px-5 py-4 text-left transition-colors hover:bg-ink/5"
                        >
                            <span className="flex items-center gap-3 font-sans text-ink">
                                <MessageCircle
                                    aria-hidden
                                    className="h-5 w-5 shrink-0 text-ink/40"
                                />
                                {thread.title}
                            </span>
                            <span className="shrink-0 font-sans text-xs text-copy-muted">
                                {formatDate(thread.updated_at)}
                            </span>
                        </button>
                    ))}
                </div>
            )}
        </div>
    );
}

function PostList({
    account,
    thread,
    onBack
}: {
    account: Account | null;
    thread: ForumThread;
    onBack: () => void;
}) {
    const [posts, setPosts] = useState<ForumPost[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [reply, setReply] = useState("");
    const [submitting, setSubmitting] = useState(false);
    const [pendingReaction, setPendingReaction] = useState<string | null>(null);

    useEffect(() => {
        let ignore = false;
        listPosts(thread.id)
            .then(({ data }) => !ignore && setPosts(data))
            .catch((cause) => !ignore && setError(describeError(cause)));
        return () => {
            ignore = true;
        };
    }, [thread.id]);

    async function handleReply(event: React.FormEvent) {
        event.preventDefault();
        setSubmitting(true);
        setError(null);
        try {
            const { data } = await createPost(thread.id, reply);
            setPosts((prev) => (prev ? [...prev, data] : [data]));
            setReply("");
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setSubmitting(false);
        }
    }

    async function toggleReaction(post: ForumPost, emoji: string) {
        if (!account) return;
        const current = post.reactions?.find((reaction) => reaction.emoji === emoji);
        const reactionKey = `${post.id}:${emoji}`;
        setPendingReaction(reactionKey);
        setError(null);
        try {
            if (current?.mine) {
                await removePostReaction(post.id, emoji);
            } else {
                await addPostReaction(post.id, emoji);
            }
            setPosts((prev) =>
                prev
                    ? prev.map((item) =>
                          item.id === post.id ? updateReaction(item, emoji, !current?.mine) : item
                      )
                    : prev
            );
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setPendingReaction(null);
        }
    }

    return (
        <div className="flex flex-col gap-6">
            <button
                type="button"
                onClick={onBack}
                className="flex w-fit items-center gap-2 font-sans text-sm text-copy-muted hover:text-ink"
            >
                <ArrowLeft aria-hidden className="h-4 w-4" />
                回到討論串列表
            </button>
            <h1 className="font-serif text-hero-subtitle text-ink">{thread.title}</h1>

            {error ? <ErrorText>{error}</ErrorText> : null}

            {posts === null ? (
                <LoadingState />
            ) : (
                <div className="flex flex-col gap-3">
                    {posts.map((post) => (
                        <div key={post.id} className={panelClass}>
                            <p className="font-sans whitespace-pre-wrap text-ink">{post.body}</p>
                            <p className="mt-3 font-sans text-xs text-copy-muted">
                                {formatDate(post.created_at)}
                            </p>
                            <div className="mt-4 flex flex-wrap gap-2">
                                {reactionOptions
                                    .filter(
                                        (emoji) =>
                                            Boolean(account) ||
                                            post.reactions?.some(
                                                (reaction) => reaction.emoji === emoji
                                            )
                                    )
                                    .map((emoji) => {
                                        const reaction = post.reactions?.find(
                                            (item) => item.emoji === emoji
                                        );
                                        const reactionKey = `${post.id}:${emoji}`;
                                        return (
                                            <button
                                                key={emoji}
                                                type="button"
                                                disabled={
                                                    !account || pendingReaction === reactionKey
                                                }
                                                aria-label={`${emoji} 反應`}
                                                aria-pressed={reaction?.mine ?? false}
                                                title={
                                                    account
                                                        ? `以 ${emoji} 反應`
                                                        : "登入後才能使用反應"
                                                }
                                                onClick={() => void toggleReaction(post, emoji)}
                                                className={twMerge(
                                                    "inline-flex items-center gap-1 rounded-full border px-2.5 py-1 font-sans text-sm transition-colors disabled:cursor-not-allowed disabled:opacity-60",
                                                    reaction?.mine
                                                        ? "border-ink/30 bg-accent-yellow/70 text-ink"
                                                        : "border-ink/10 bg-surface text-ink/70 hover:bg-ink/5"
                                                )}
                                            >
                                                <span aria-hidden>{emoji}</span>
                                                <span>{reaction?.count ?? 0}</span>
                                            </button>
                                        );
                                    })}
                            </div>
                        </div>
                    ))}
                </div>
            )}

            {account && account.identity_status !== "temporary" ? (
                <form className="flex flex-col gap-3" onSubmit={handleReply}>
                    <textarea
                        className={twMerge(inputClass, "min-h-24 resize-y")}
                        placeholder="回覆這則討論…"
                        required
                        value={reply}
                        onChange={(e) => setReply(e.target.value)}
                    />
                    <Button type="submit" disabled={submitting} className="w-fit px-6">
                        {submitting ? "送出中…" : "回覆"}
                    </Button>
                </form>
            ) : account ? (
                <p className="font-sans text-sm text-copy-muted">
                    你的帳號尚未完成身份驗證，暫時無法回覆討論。
                </p>
            ) : (
                <p className="font-sans text-sm text-copy-muted">
                    <a href="/login" className="underline">
                        登入
                    </a>{" "}
                    後才能回覆討論。
                </p>
            )}
        </div>
    );
}

function formatDate(iso: string): string {
    try {
        return new Date(iso).toLocaleString("zh-TW", { dateStyle: "medium", timeStyle: "short" });
    } catch {
        return iso;
    }
}

function updateReaction(post: ForumPost, emoji: string, mine: boolean): ForumPost {
    const reactions = [...(post.reactions ?? [])];
    const index = reactions.findIndex((reaction) => reaction.emoji === emoji);

    if (mine) {
        if (index === -1) {
            reactions.push({ emoji, count: 1, mine: true });
        } else {
            reactions[index] = {
                ...reactions[index],
                count: reactions[index].count + 1,
                mine: true
            };
        }
    } else if (index !== -1) {
        if (reactions[index].count <= 1) {
            reactions.splice(index, 1);
        } else {
            reactions[index] = {
                ...reactions[index],
                count: reactions[index].count - 1,
                mine: false
            };
        }
    }

    return { ...post, reactions };
}
