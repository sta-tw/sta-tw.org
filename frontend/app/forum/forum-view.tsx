"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import { ArrowLeft, MessageCircle } from "lucide-react";
import { twMerge } from "tailwind-merge";
import Button from "../components/button";
import { Card } from "../components/card";
import { Input, Textarea } from "../components/input";
import { EmptyState, ErrorText, LoadingState } from "../components/status";
import { describeError } from "../lib/describe-error";
import { formatDateTime } from "../lib/format";
import { useCurrentAccount } from "../lib/current-account";
import { loginHref } from "../lib/routes";
import { useAction } from "../lib/use-resource";
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
import type { Account } from "../lib/api/types";

type View = { name: "threads" } | { name: "posts"; thread: ForumThread };

const reactionOptions = ["👍", "❤️", "🎉"];

export default function ForumView() {
    const account = useCurrentAccount().me?.account ?? null;
    const [space, setSpace] = useState<ForumSpace | null>(null);
    const [spaceError, setSpaceError] = useState<string | null>(null);
    const [view, setView] = useState<View>({ name: "threads" });

    useEffect(() => {
        let ignore = false;
        listSpaces()
            .then(({ data }) => {
                if (ignore) return;
                // Exactly one `global` space is guaranteed to exist (seeded by
                // migration 000013); that's the only entry point this page
                // exposes today, so jump straight into it instead of making
                // people pick from a list of one.
                const globalSpace =
                    data.find((candidate) => candidate.space_type === "global") ?? null;
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
            <Card>
                <ErrorText>{spaceError}</ErrorText>
            </Card>
        );
    }
    if (!space) {
        return <LoadingState label="討論區載入中" />;
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
    const { pending, error: actionError, run } = useAction();
    const submitting = pending !== null;

    useEffect(() => {
        let ignore = false;
        listThreads(space.id)
            .then(({ data }) => !ignore && setThreads(data))
            .catch((cause) => !ignore && setError(describeError(cause)));
        return () => {
            ignore = true;
        };
    }, [space.id]);

    function handleCreate(event: React.FormEvent) {
        event.preventDefault();
        void run(async () => {
            const { thread } = await createThread(space.id, form);
            setThreads((prev) => (prev ? [thread, ...prev] : [thread]));
            setForm({ title: "", body: "" });
            setShowForm(false);
        });
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

            <ErrorText>{error ?? actionError}</ErrorText>

            {showForm ? (
                <Card>
                    <form className="flex flex-col gap-4" onSubmit={handleCreate}>
                        <Input
                            aria-label="標題"
                            placeholder="標題"
                            required
                            maxLength={300}
                            value={form.title}
                            onChange={(e) => setForm((f) => ({ ...f, title: e.target.value }))}
                        />
                        <Textarea
                            aria-label="內容"
                            placeholder="內容"
                            required
                            value={form.body}
                            onChange={(e) => setForm((f) => ({ ...f, body: e.target.value }))}
                        />
                        <Button type="submit" disabled={submitting} className="w-fit px-6">
                            {submitting ? "送出中…" : "送出討論"}
                        </Button>
                    </form>
                </Card>
            ) : null}

            {threads === null ? (
                <LoadingState />
            ) : threads.length === 0 ? (
                <EmptyState title="這裡還沒有討論串，當第一個發文的人吧！" />
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
                                {formatDateTime(thread.updated_at)}
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
    const pathname = usePathname();
    const [posts, setPosts] = useState<ForumPost[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [reply, setReply] = useState("");
    const { pending, error: actionError, run } = useAction();

    useEffect(() => {
        let ignore = false;
        listPosts(thread.id)
            .then(({ data }) => !ignore && setPosts(data))
            .catch((cause) => !ignore && setError(describeError(cause)));
        return () => {
            ignore = true;
        };
    }, [thread.id]);

    function handleReply(event: React.FormEvent) {
        event.preventDefault();
        void run("reply", async () => {
            const { data } = await createPost(thread.id, reply);
            setPosts((prev) => (prev ? [...prev, data] : [data]));
            setReply("");
        });
    }

    function toggleReaction(post: ForumPost, emoji: string) {
        if (!account) return;
        const current = post.reactions?.find((reaction) => reaction.emoji === emoji);
        void run(`${post.id}:${emoji}`, async () => {
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
        });
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

            <ErrorText>{error ?? actionError}</ErrorText>

            {posts === null ? (
                <LoadingState />
            ) : (
                <div className="flex flex-col gap-3">
                    {posts.map((post) => (
                        <Card key={post.id}>
                            <p className="font-sans whitespace-pre-wrap text-ink">{post.body}</p>
                            <p className="mt-3 font-sans text-xs text-copy-muted">
                                {formatDateTime(post.created_at)}
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
                                                disabled={!account || pending === reactionKey}
                                                aria-label={`${emoji} 反應`}
                                                aria-pressed={reaction?.mine ?? false}
                                                title={
                                                    account
                                                        ? `以 ${emoji} 反應`
                                                        : "登入後才能使用反應"
                                                }
                                                onClick={() => toggleReaction(post, emoji)}
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
                        </Card>
                    ))}
                </div>
            )}

            {account && account.identity_status !== "temporary" ? (
                <form className="flex flex-col gap-3" onSubmit={handleReply}>
                    <Textarea
                        aria-label="回覆內容"
                        className="min-h-24"
                        placeholder="回覆這則討論…"
                        required
                        value={reply}
                        onChange={(e) => setReply(e.target.value)}
                    />
                    <Button type="submit" disabled={pending === "reply"} className="w-fit px-6">
                        {pending === "reply" ? "送出中…" : "回覆"}
                    </Button>
                </form>
            ) : account ? (
                <p className="font-sans text-sm text-copy-muted">
                    你的帳號尚未完成身份驗證，暫時無法回覆討論。
                </p>
            ) : (
                <p className="font-sans text-sm text-copy-muted">
                    <Link href={loginHref(pathname)} className="underline">
                        登入
                    </Link>{" "}
                    後才能回覆討論。
                </p>
            )}
        </div>
    );
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
