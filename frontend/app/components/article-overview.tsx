"use client";

import Image from "next/image";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { ChevronLeft, ChevronRight, Search, X } from "lucide-react";
import { Suspense, useEffect, useMemo, useState, type FormEvent } from "react";
import { twMerge } from "tailwind-merge";
import { readingTimeOf } from "../article/article-format";
import { formatNumericDate } from "../lib/format";
import { listExperienceTags, listExperiences } from "../lib/api/content";
import {
    articleOverviewFallbackPath,
    getArticleOverviewImageMeta,
    getArticleOverviewImageUrl
} from "../lib/api/site-assets";
import { publicPath } from "../lib/public-path";
import { articleOverviewHref, experienceHref } from "../lib/routes";
import { normalizeSearchText } from "../lib/text-normalize";
import { useResource } from "../lib/use-resource";
import { ContentCard } from "./card";
import FaqAccordion from "./faq-accordion";
import HashtagBadge from "./hashtag-badge";
import IconButton from "./icon-button";
import { EmptyState, LoadingState, Resource } from "./status";

const fallbackArticleOverviewImage = publicPath(articleOverviewFallbackPath);

const CLOUD_TAG_LIMIT = 12;
// Smallest to largest; a tag's size reflects how many articles use it.
const tagSizes = [
    "text-xl sm:text-2xl",
    "text-2xl sm:text-3xl",
    "text-3xl sm:text-4xl",
    "text-4xl sm:text-5xl"
];
// Hand-tuned offsets cycled by index for the "scattered word cloud" look.
const tagPositions = [
    "self-start ml-[22%]",
    "self-end mr-[28%] -mt-2",
    "self-center -mt-2",
    "self-start ml-[12%] -mt-1",
    "self-end mr-[15%] -mt-5",
    "self-center -mt-1",
    "self-end mr-[13%] -mt-3",
    "self-center -mt-1",
    "self-end mr-[18%] -mt-5"
];

type TagCount = { tag: string; count: number };

function tagSizeClass(count: number, min: number, max: number) {
    if (max === min) return tagSizes[1];
    const level = Math.round(((count - min) / (max - min)) * (tagSizes.length - 1));
    return tagSizes[level];
}

const searchable = (text: string) => normalizeSearchText(text.toLowerCase());

// The backend only pages, so the overview loads one page and filters by
// ?tag= / ?q= client-side; each item's search text is normalised once.
const loadArticles = (signal: AbortSignal) =>
    listExperiences({ limit: 100, signal }).then(({ data }) =>
        data.map((article) => ({ article, text: searchable(`${article.title}\n${article.body}`) }))
    );

function excerptOf(body: string) {
    const text = body.trim().replace(/\s+/g, " ");
    return text.length > 200 ? `${text.slice(0, 200)}…` : text || "查看這篇特殊選才經驗分享。";
}

export default function ArticleOverview() {
    const [activeSlide, setActiveSlide] = useState(0);
    const [featuredImage, setFeaturedImage] = useState(fallbackArticleOverviewImage);

    useEffect(() => {
        let ignore = false;
        getArticleOverviewImageMeta()
            .then(({ data }) => {
                if (!ignore && data.exists) {
                    setFeaturedImage(getArticleOverviewImageUrl(data.updated_at));
                }
            })
            .catch(() => {
                // The bundled PNG remains the fallback while the API is unavailable.
            });
        return () => {
            ignore = true;
        };
    }, []);

    const slides = useMemo(
        () => [
            {
                image: featuredImage,
                alt: "S.T.A 特殊選才資源網文章總覽主視覺"
            }
        ],
        [featuredImage]
    );

    const changeSlide = (direction: -1 | 1) => {
        setActiveSlide((current) => (current + direction + slides.length) % slides.length);
    };

    return (
        <main className="max-w-full min-w-0 flex-1 overflow-x-clip bg-surface">
            <div className="mx-auto w-full max-w-screen-xl min-w-0 px-5 pt-7 pb-16 sm:px-6 sm:pt-10 sm:pb-20 lg:px-16 lg:pt-16 lg:pb-24">
                <section aria-label="精選文章" className="w-full max-w-full overflow-hidden">
                    <div className="relative aspect-[1891/831] w-full max-w-full overflow-hidden rounded-[15%] bg-surface">
                        {slides.map((slide, index) => (
                            <Image
                                key={index}
                                src={slide.image}
                                alt={slide.alt}
                                fill
                                priority={index === 0}
                                unoptimized={slide.image !== fallbackArticleOverviewImage}
                                sizes="(min-width: 1280px) 1152px, (min-width: 640px) calc(100vw - 48px), calc(100vw - 40px)"
                                className={twMerge(
                                    "object-contain transition-opacity duration-500",
                                    index === activeSlide ? "opacity-100" : "opacity-0"
                                )}
                            />
                        ))}

                        {slides.length > 1 ? (
                            <>
                                <button
                                    type="button"
                                    onClick={() => changeSlide(-1)}
                                    className="absolute top-1/2 left-4 inline-flex h-11 w-11 -translate-y-1/2 items-center justify-center rounded-full bg-accent-yellow text-ink shadow-sm transition-transform hover:scale-105 focus-visible:ring-2 focus-visible:ring-surface focus-visible:ring-offset-2 focus-visible:outline-none sm:left-6 sm:h-14 sm:w-14"
                                    aria-label="上一張精選文章"
                                >
                                    <ChevronLeft aria-hidden className="h-7 w-7 stroke-[3]" />
                                </button>
                                <button
                                    type="button"
                                    onClick={() => changeSlide(1)}
                                    className="absolute top-1/2 right-4 inline-flex h-11 w-11 -translate-y-1/2 items-center justify-center rounded-full bg-accent-yellow text-ink shadow-sm transition-transform hover:scale-105 focus-visible:ring-2 focus-visible:ring-surface focus-visible:ring-offset-2 focus-visible:outline-none sm:right-6 sm:h-14 sm:w-14"
                                    aria-label="下一張精選文章"
                                >
                                    <ChevronRight aria-hidden className="h-7 w-7 stroke-[3]" />
                                </button>

                                <div
                                    className="absolute right-0 bottom-4 left-0 flex justify-center gap-2"
                                    aria-label="精選文章頁數"
                                >
                                    {slides.map((_, index) => (
                                        <button
                                            key={index}
                                            type="button"
                                            onClick={() => setActiveSlide(index)}
                                            aria-label={`前往第 ${index + 1} 張精選文章`}
                                            aria-current={
                                                index === activeSlide ? "true" : undefined
                                            }
                                            className={twMerge(
                                                "h-3 w-3 rounded-full border border-surface/50 transition-colors",
                                                index === activeSlide
                                                    ? "bg-accent-yellow"
                                                    : "bg-surface/80 hover:bg-surface"
                                            )}
                                        />
                                    ))}
                                </div>
                            </>
                        ) : null}
                    </div>
                </section>

                <div className="article-dots mt-0 px-4 py-7 sm:px-8 sm:py-10 lg:px-10">
                    <Suspense fallback={<LoadingState label="正在載入公開心得" />}>
                        <ArticleExplorer />
                    </Suspense>

                    <section aria-labelledby="article-faq-title" className="mt-20 sm:mt-24">
                        <h2
                            id="article-faq-title"
                            className="font-serif text-3xl text-ink sm:text-4xl"
                        >
                            常見問題
                        </h2>
                        <div className="mt-8 sm:mt-10">
                            <FaqAccordion />
                        </div>
                        <div className="mt-8 text-right sm:mt-10">
                            <Link
                                href="/faq"
                                className="text-base font-medium text-ink underline-offset-4 focus-ring hover:underline sm:text-lg"
                            >
                                &gt;&gt; 查看完整常見問題
                            </Link>
                        </div>
                    </section>
                </div>
            </div>
        </main>
    );
}

/** Search bar, filtered list and tag cloud, all driven by ?tag= / ?q=. */
function ArticleExplorer() {
    const router = useRouter();
    const searchParams = useSearchParams();
    const tag = searchParams.get("tag")?.trim() ?? "";
    const q = searchParams.get("q")?.trim() ?? "";
    const { state, reload } = useResource(loadArticles);
    const [tags, setTags] = useState<TagCount[]>([]);

    const results = useMemo(() => {
        if (state.status !== "ready") return [];
        const query = searchable(q);
        return state.data
            .filter(
                (item) =>
                    (!tag || item.article.tags.includes(tag)) &&
                    (!query || item.text.includes(query))
            )
            .map((item) => item.article);
    }, [state, tag, q]);

    useEffect(() => {
        let ignore = false;
        listExperienceTags()
            .then(({ data }) => {
                if (!ignore) setTags(data);
            })
            .catch(() => {
                // The cloud simply stays hidden without tag data.
            });
        return () => {
            ignore = true;
        };
    }, []);

    const cloud = useMemo(() => {
        const top = tags.slice(0, CLOUD_TAG_LIMIT);
        const counts = top.map((item) => item.count);
        const min = Math.min(...counts);
        const max = Math.max(...counts);
        // Alphabetical order scatters the big tags instead of stacking them on top.
        return top
            .map((item) => ({ ...item, size: tagSizeClass(item.count, min, max) }))
            .sort((a, b) => a.tag.localeCompare(b.tag, "zh-TW"));
    }, [tags]);

    const onSearch = (event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        const value = String(new FormData(event.currentTarget).get("q") ?? "").trim();
        router.push(articleOverviewHref({ tag, q: value }), { scroll: false });
    };

    return (
        <>
            <form
                role="search"
                key={q}
                onSubmit={onSearch}
                className="flex h-12 w-full items-center gap-1 rounded-[var(--radius-small)] border-2 border-accent-yellow-strong bg-accent-yellow/70 pr-4 pl-1 text-ink shadow-sm focus-within:ring-2 focus-within:ring-ink sm:mx-auto sm:max-w-4xl"
            >
                <IconButton
                    type="submit"
                    label="搜尋"
                    icon={<Search className="h-6 w-6 stroke-[2.5]" />}
                    className="h-10 w-10 shrink-0 hover:scale-105 focus-visible:outline-none"
                />
                <label htmlFor="article-search" className="sr-only">
                    搜尋文章
                </label>
                <input
                    id="article-search"
                    name="q"
                    type="search"
                    defaultValue={q}
                    placeholder="嘗試搜尋「特選心得」"
                    className="min-w-0 flex-1 bg-transparent font-sans text-base text-ink placeholder:text-ink/60 focus:outline-none sm:text-lg"
                />
            </form>

            {(tag || q) && (
                <div className="mx-auto mt-5 flex max-w-4xl flex-wrap items-center gap-2 font-sans text-sm text-copy-muted sm:text-base">
                    <span>目前篩選：</span>
                    {tag && (
                        <FilterChip
                            label={`#${tag}`}
                            clearLabel={`清除標籤篩選 #${tag}`}
                            href={articleOverviewHref({ q })}
                        />
                    )}
                    {q && (
                        <FilterChip
                            label={`「${q}」`}
                            clearLabel={`清除搜尋「${q}」`}
                            href={articleOverviewHref({ tag })}
                        />
                    )}
                </div>
            )}

            <section
                aria-labelledby="article-list-title"
                className="mx-auto mt-8 flex max-w-6xl flex-col gap-5 sm:mt-10 sm:gap-6"
            >
                <h2 id="article-list-title" className="sr-only">
                    文章列表
                </h2>
                <Resource state={state} reload={reload} loadingLabel="正在從後端載入公開心得">
                    {() =>
                        results.length ? (
                            results.map((article) => (
                                <ContentCard
                                    key={article.id}
                                    href={experienceHref(article.id)}
                                    title={article.title}
                                    excerpt={excerptOf(article.body)}
                                    thumbnailSeed={article.id}
                                    meta={
                                        <>
                                            <time dateTime={article.updated_at}>
                                                {formatNumericDate(article.updated_at)}
                                            </time>
                                            <span>閱讀時間: {readingTimeOf(article.body)}</span>
                                            {article.tags.slice(0, 3).map((item) => (
                                                <HashtagBadge key={item} tag={item} />
                                            ))}
                                        </>
                                    }
                                />
                            ))
                        ) : (
                            <EmptyState
                                title={
                                    tag || q
                                        ? "找不到符合篩選條件的文章。"
                                        : "目前沒有可公開的心得文章。"
                                }
                            />
                        )
                    }
                </Resource>
            </section>

            {cloud.length > 0 && (
                <section aria-labelledby="hashtags-title" className="mt-16 sm:mt-20">
                    <h2 id="hashtags-title" className="font-serif text-3xl text-ink sm:text-4xl">
                        Hashtags
                    </h2>
                    <ul className="mx-auto mt-5 flex max-w-2xl flex-col items-center gap-1 text-center sm:mt-7">
                        {cloud.map((item, index) => (
                            <li
                                key={item.tag}
                                className={tagPositions[index % tagPositions.length]}
                            >
                                <Link
                                    href={articleOverviewHref({ tag: item.tag, q })}
                                    scroll={false}
                                    aria-current={item.tag === tag ? "true" : undefined}
                                    className={twMerge(
                                        "inline-block rounded-[var(--radius-small)] bg-accent-yellow/80 px-2 py-0.5 font-sans leading-tight text-ink focus-ring transition-transform hover:scale-105 aria-[current]:bg-accent-green aria-[current]:ring-2 aria-[current]:ring-ink",
                                        item.size
                                    )}
                                >
                                    #{item.tag}
                                    <span className="sr-only">，{item.count} 篇文章</span>
                                </Link>
                            </li>
                        ))}
                    </ul>
                </section>
            )}
        </>
    );
}

function FilterChip({
    label,
    clearLabel,
    href
}: {
    label: string;
    clearLabel: string;
    href: string;
}) {
    return (
        <span className="inline-flex items-center gap-1 rounded-full bg-accent-green/55 py-1 pr-1 pl-3 font-semibold text-ink">
            {label}
            <Link
                href={href}
                scroll={false}
                aria-label={clearLabel}
                className="inline-flex size-6 items-center justify-center rounded-full focus-ring hover:bg-ink/10"
            >
                <X aria-hidden className="size-4" />
            </Link>
        </span>
    );
}
