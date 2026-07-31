"use client";

import Image from "next/image";
import Link from "next/link";
import { ChevronLeft, ChevronRight, Search } from "lucide-react";
import { useMemo, useState } from "react";
import { twMerge } from "tailwind-merge";
import { articles, type Article } from "../data/articles";

const slides = [
    {
        image: "/articlemain/starlit-boat-hero.png",
        alt: "小船載著展翅的學生航行在星空與海面之間"
    },
    {
        image: "/articlemain/starlit-boat-hero.png",
        alt: "小船載著展翅的學生航行在星空與海面之間"
    },
    {
        image: "/articlemain/starlit-boat-hero.png",
        alt: "小船載著展翅的學生航行在星空與海面之間"
    }
];

const tagCloud = [
    { label: "成大", size: "text-xl sm:text-2xl", position: "self-start ml-[22%]" },
    { label: "作品集", size: "text-xl sm:text-2xl", position: "self-end mr-[28%] -mt-2" },
    { label: "輔導", size: "text-2xl sm:text-3xl", position: "self-center -mt-2" },
    { label: "特選心得", size: "text-xl sm:text-2xl", position: "self-start ml-[12%] -mt-1" },
    { label: "面試", size: "text-2xl sm:text-3xl", position: "self-end mr-[15%] -mt-5" },
    { label: "資工", size: "text-2xl sm:text-3xl", position: "self-center -mt-1" },
    { label: "備審", size: "text-4xl sm:text-5xl", position: "self-center -mt-2" },
    { label: "美術", size: "text-xl sm:text-2xl", position: "self-end mr-[13%] -mt-3" },
    { label: "中文系", size: "text-xl sm:text-2xl", position: "self-center -mt-1" },
    { label: "不分系", size: "text-3xl sm:text-4xl", position: "self-end mr-[18%] -mt-5" }
];

export default function ArticleOverview() {
    const [activeSlide, setActiveSlide] = useState(0);
    const [query, setQuery] = useState("");

    const filteredArticles = useMemo(() => {
        const normalizedQuery = query.trim().toLowerCase();
        if (!normalizedQuery) return articles;

        return articles.filter((article) =>
            [article.title, article.summary, ...article.tags]
                .join(" ")
                .toLowerCase()
                .includes(normalizedQuery)
        );
    }, [query]);

    const changeSlide = (direction: -1 | 1) => {
        setActiveSlide((current) => (current + direction + slides.length) % slides.length);
    };

    return (
        <main className="flex-1 bg-surface">
            <div className="mx-auto w-full max-w-screen-xl px-5 pt-7 pb-16 sm:px-6 sm:pt-10 sm:pb-20 lg:px-16 lg:pt-16 lg:pb-24">
                <section aria-label="精選文章" className="overflow-hidden">
                    <div className="relative aspect-[16/8] min-h-72 overflow-hidden bg-ink sm:aspect-[16/7]">
                        {slides.map((slide, index) => (
                            <Image
                                key={index}
                                src={slide.image}
                                alt={slide.alt}
                                fill
                                priority={index === 0}
                                sizes="(min-width: 1280px) 1152px, (min-width: 640px) calc(100vw - 48px), calc(100vw - 40px)"
                                className={twMerge(
                                    "object-cover transition-opacity duration-500",
                                    index === activeSlide ? "opacity-100" : "opacity-0"
                                )}
                            />
                        ))}

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
                                    aria-current={index === activeSlide ? "true" : undefined}
                                    className={twMerge(
                                        "h-3 w-3 rounded-full border border-surface/50 transition-colors",
                                        index === activeSlide
                                            ? "bg-accent-yellow"
                                            : "bg-surface/80 hover:bg-surface"
                                    )}
                                />
                            ))}
                        </div>
                    </div>
                </section>

                <div className="article-dots mt-0 px-4 py-7 sm:px-8 sm:py-10 lg:px-10">
                    <label className="flex h-12 w-full items-center gap-3 rounded-[var(--radius-small)] border-2 border-[#ffc34e] bg-accent-yellow/70 px-4 text-ink shadow-sm sm:mx-auto sm:max-w-4xl">
                        <Search aria-hidden className="h-6 w-6 shrink-0 stroke-[2.5]" />
                        <span className="sr-only">搜尋文章</span>
                        <input
                            value={query}
                            onChange={(event) => setQuery(event.target.value)}
                            placeholder="嘗試搜尋「特選心得」"
                            className="min-w-0 flex-1 bg-transparent font-sans text-base text-ink placeholder:text-ink/60 focus:outline-none sm:text-lg"
                        />
                    </label>

                    <section
                        aria-label="文章列表"
                        className="mx-auto mt-8 flex max-w-6xl flex-col gap-5 sm:mt-10 sm:gap-6"
                    >
                        {filteredArticles.length ? (
                            filteredArticles.map((article) => (
                                <ArticleCard key={article.title} article={article} />
                            ))
                        ) : (
                            <p className="rounded-[var(--radius-small)] bg-ink/5 px-5 py-12 text-center text-lg text-ink/70">
                                找不到符合「{query}」的文章。
                            </p>
                        )}
                    </section>

                    <section aria-labelledby="hashtags-title" className="mt-16 sm:mt-20">
                        <h2
                            id="hashtags-title"
                            className="font-serif text-3xl text-ink sm:text-4xl"
                        >
                            Hashtags
                        </h2>
                        <div className="mx-auto mt-5 flex max-w-2xl flex-col items-center gap-1 text-center sm:mt-7">
                            {tagCloud.map((tag) => (
                                <button
                                    key={tag.label}
                                    type="button"
                                    onClick={() => setQuery(tag.label)}
                                    className={twMerge(
                                        "rounded-[var(--radius-small)] bg-accent-yellow/80 px-2 py-0.5 font-sans leading-tight text-ink transition-transform hover:scale-105 focus-visible:ring-2 focus-visible:ring-ink focus-visible:outline-none",
                                        tag.size,
                                        tag.position,
                                        tag.label === "備審" && "bg-[#ffc34e] px-3 py-1"
                                    )}
                                >
                                    #{tag.label}
                                </button>
                            ))}
                        </div>
                    </section>

                    <section
                        aria-labelledby="article-faq-title"
                        className="relative mt-20 min-h-80 sm:mt-24 sm:min-h-96 lg:min-h-[31rem]"
                    >
                        <h2
                            id="article-faq-title"
                            className="font-serif text-3xl text-ink sm:text-4xl"
                        >
                            常見問題
                        </h2>
                        <Link
                            href="/faq"
                            className="absolute right-0 bottom-0 text-base font-medium text-ink underline-offset-4 hover:underline focus-visible:ring-2 focus-visible:ring-ink focus-visible:outline-none sm:text-lg"
                        >
                            &gt;&gt; 查看完整常見問題
                        </Link>
                    </section>
                </div>
            </div>
        </main>
    );
}

function ArticleCard({ article }: { article: Article }) {
    return (
        <Link
            href={`/article/${article.slug}`}
            className="block rounded-[var(--radius-small)] focus-visible:ring-2 focus-visible:ring-ink focus-visible:ring-offset-2 focus-visible:outline-none"
            aria-label={`閱讀文章：${article.title}`}
        >
            <article className="grid gap-5 rounded-[var(--radius-small)] bg-ink/5 p-5 transition-colors hover:bg-ink/10 sm:grid-cols-[12rem_minmax(0,1fr)] sm:items-stretch sm:gap-7 sm:p-7 lg:grid-cols-[15.5rem_minmax(0,1fr)]">
                <div
                    aria-hidden
                    className={twMerge(
                        "min-h-40 rounded-[var(--radius-small)] sm:min-h-full",
                        article.accent === "green" ? "bg-accent-green" : "bg-accent-yellow"
                    )}
                />
                <div className="flex min-w-0 flex-col">
                    <h2 className="font-sans text-xl leading-snug font-medium text-ink sm:text-2xl lg:text-3xl">
                        {article.listingTitle}
                    </h2>
                    <p className="mt-4 text-base leading-relaxed text-ink/80 sm:text-lg">
                        {article.summary}
                    </p>
                    <time
                        dateTime={article.date}
                        className="mt-6 self-end text-base font-medium text-ink/80 sm:text-lg"
                    >
                        {article.date.replaceAll("-", " - ")}
                    </time>
                </div>
            </article>
        </Link>
    );
}
