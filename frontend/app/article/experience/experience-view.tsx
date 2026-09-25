"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";
import { getExperience, type Experience } from "../../lib/api/content";
import { ApiError } from "../../lib/api/types";

type ExperienceViewState = {
    id: string;
    experience: Experience | null;
    error: string | null;
};

export default function ExperienceView() {
    const searchParams = useSearchParams();
    const id = searchParams.get("id")?.trim() ?? "";
    const [state, setState] = useState<ExperienceViewState>({
        id: "",
        experience: null,
        error: null
    });

    useEffect(() => {
        const controller = new AbortController();

        if (!id) return () => controller.abort();

        getExperience(id, { signal: controller.signal })
            .then(({ data }) => {
                if (controller.signal.aborted) return;
                setState({ id, experience: data, error: null });
            })
            .catch((cause: unknown) => {
                if (controller.signal.aborted) return;
                setState({
                    id,
                    experience: null,
                    error:
                        cause instanceof ApiError && cause.status === 404
                            ? "找不到這篇已公開的心得文章。"
                            : cause instanceof ApiError
                              ? cause.message
                              : "目前無法取得心得文章，請稍後再試。"
                });
            });

        return () => controller.abort();
    }, [id]);

    const isLoading = Boolean(id) && state.id !== id;
    const experience = state.id === id ? state.experience : null;
    const error = state.id === id ? state.error : null;

    if (!id) {
        return <ArticleMessage message="缺少文章識別碼。" />;
    }

    if (isLoading) {
        return <ArticleMessage message="正在從後端載入心得文章…" />;
    }

    if (error || !experience) {
        return (
            <ArticleMessage title="無法載入心得文章" message={error ?? "目前沒有可顯示的資料。"} />
        );
    }

    return (
        <main className="article-dots flex-1 bg-surface">
            <article className="mx-auto w-full max-w-screen-lg px-5 pt-12 pb-16 sm:px-6 sm:pt-16 sm:pb-20 lg:px-16 lg:pt-20 lg:pb-24">
                <Link
                    href="/articlemain"
                    className="font-sans text-sm text-ink/65 underline-offset-4 hover:text-ink hover:underline sm:text-base"
                >
                    ← 回到文章總覽
                </Link>

                <header className="mt-6 border-b border-ink/10 pb-8 text-center sm:mt-8 sm:pb-10">
                    <h1 className="font-sans text-3xl leading-tight font-medium tracking-[-0.03em] text-ink sm:text-4xl lg:text-5xl">
                        {experience.title}
                    </h1>
                    <div className="mt-5 flex flex-wrap items-center justify-center gap-x-5 gap-y-2 font-sans text-sm text-ink/70 sm:text-base">
                        {experience.author_type !== "-" ? (
                            <span>身份：{experience.author_type}</span>
                        ) : null}
                        {experience.admission_outcome !== "-" ? (
                            <span>結果：{experience.admission_outcome}</span>
                        ) : null}
                        <time dateTime={experience.updated_at}>
                            Update Date : {formatDate(experience.updated_at)}
                        </time>
                    </div>
                </header>

                <div className="mx-auto mt-10 max-w-3xl font-sans text-base leading-8 text-ink/85 sm:mt-12 sm:text-lg sm:leading-9">
                    {toBodyBlocks(experience.body).map((block, index) => (
                        <section
                            key={`${block.type}-${index}`}
                            className="mb-10 last:mb-0 sm:mb-12"
                        >
                            {block.type === "list" ? (
                                <ul className="list-disc space-y-2 pl-6 marker:text-ink/60">
                                    {block.items.map((item) => (
                                        <li key={item}>{item}</li>
                                    ))}
                                </ul>
                            ) : (
                                <p className="whitespace-pre-wrap">{block.text}</p>
                            )}
                        </section>
                    ))}
                </div>
            </article>
        </main>
    );
}

function ArticleMessage({ title = "心得文章", message }: { title?: string; message: string }) {
    return (
        <main className="article-dots flex flex-1 items-center justify-center bg-surface px-5 py-20">
            <div className="max-w-md text-center">
                <p className="font-sans text-lg font-medium text-ink">{title}</p>
                <p className="mt-2 font-sans text-base leading-7 text-ink/65">{message}</p>
                <Link
                    href="/articlemain"
                    className="mt-6 inline-flex rounded-full bg-button px-4 py-2 font-sans text-sm font-medium text-button-foreground hover:bg-button-hover"
                >
                    返回文章總覽
                </Link>
            </div>
        </main>
    );
}

type BodyBlock = { type: "paragraph"; text: string } | { type: "list"; items: string[] };

function toBodyBlocks(body: string): BodyBlock[] {
    return body
        .trim()
        .split(/\n\s*\n/)
        .map((block) => block.trim())
        .filter(Boolean)
        .map((block) => {
            const lines = block
                .split(/\r?\n/)
                .map((line) => line.trim())
                .filter(Boolean);
            const isList = lines.length > 0 && lines.every((line) => /^[-*]\s+/.test(line));

            return isList
                ? { type: "list", items: lines.map((line) => line.replace(/^[-*]\s+/, "")) }
                : { type: "paragraph", text: lines.join("\n") };
        });
}

function formatDate(iso: string): string {
    try {
        return new Date(iso).toLocaleDateString("zh-TW");
    } catch {
        return iso.slice(0, 10);
    }
}
