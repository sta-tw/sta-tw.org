import type { Metadata } from "next";
import Image from "next/image";
import Link from "next/link";
import { notFound } from "next/navigation";
import { Separator } from "radix-ui";
import { twMerge } from "tailwind-merge";
import HashtagBadge from "../../components/hashtag-badge";
import { getArticleById, getArticleIds, type Article } from "../../lib/article-list";

type ArticlePageProps = {
    params: Promise<{
        id: string;
    }>;
};

export function generateStaticParams() {
    return getArticleIds().map((id) => ({ id }));
}

export async function generateMetadata({ params }: ArticlePageProps): Promise<Metadata> {
    const { id } = await params;
    const article = getArticleById(id);

    if (!article) {
        return {
            title: "找不到文章 | S.T.A 特殊選才資源網"
        };
    }

    return {
        title: `${article.title} | S.T.A 特殊選才資源網`,
        description: article.excerpt
    };
}

export default async function ArticlePage({ params }: ArticlePageProps) {
    const { id } = await params;
    const article = getArticleById(id);

    if (!article) {
        notFound();
    }

    return (
        <main className="bg-surface">
            <article className="mx-auto max-w-screen-xl px-5 py-10 sm:px-6 sm:py-14 lg:px-16 lg:py-20">
                <header className="space-y-8">
                    <span className="inline-flex rounded-full bg-accent-green/65 px-4 py-1.5 text-sm font-semibold text-ink">
                        {article.category}
                    </span>

                    <div className="grid gap-8 lg:grid-cols-[minmax(0,0.92fr)_minmax(420px,1fr)] lg:items-start lg:gap-12">
                        <div className="max-w-3xl space-y-6">
                            <div className="space-y-4">
                                <h1 className="font-serif text-4xl leading-tight text-balance text-ink sm:text-5xl lg:text-[3.75rem]">
                                    {article.title}
                                </h1>
                                <p className="font-sans text-lg leading-8 font-medium text-copy-muted sm:text-xl sm:leading-9">
                                    {article.excerpt}
                                </p>
                            </div>

                            <ArticleMeta article={article} />
                        </div>

                        <ArticleCover article={article} priority />
                    </div>
                </header>

                <div className="mt-12 max-w-3xl space-y-10 lg:mt-16">
                    {article.content.map((section) => (
                        <section key={section.heading} className="space-y-4">
                            <h2 className="font-serif text-3xl leading-tight text-ink sm:text-[2rem]">
                                {section.heading}
                            </h2>
                            <div className="space-y-5">
                                {section.paragraphs.map((paragraph) => (
                                    <p
                                        key={paragraph}
                                        className="font-sans text-lg leading-9 font-medium text-copy-muted"
                                    >
                                        {paragraph}
                                    </p>
                                ))}
                            </div>
                        </section>
                    ))}
                </div>

                <footer className="mt-12 flex max-w-3xl flex-col gap-6 border-t border-ink/10 pt-8 lg:mt-16">
                    <div className="flex flex-wrap gap-2">
                        {article.tags.map((tag) => (
                            <HashtagBadge key={tag} tag={tag} />
                        ))}
                    </div>

                    <Link
                        href="/articles"
                        className="inline-flex self-start text-sm font-semibold text-copy-muted transition-colors hover:text-ink focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ink"
                    >
                        返回文章列表
                    </Link>
                </footer>
            </article>
        </main>
    );
}

function ArticleMeta({ article }: { article: Article }) {
    return (
        <div className="flex flex-wrap items-center gap-3 text-sm font-semibold text-copy-muted sm:text-base">
            <span>發布日期 {article.publishedAt}</span>
            <Separator.Root decorative orientation="vertical" className="h-4 w-px bg-ink/15" />
            <span>閱讀時間 {article.readTime}</span>
        </div>
    );
}

function ArticleCover({ article, priority = false }: { article: Article; priority?: boolean }) {
    return (
        <div className="relative aspect-[16/10] overflow-hidden rounded-[var(--radius-panel)] border border-ink/10 shadow-[var(--shadow-card)]">
            <div className={twMerge("absolute inset-0 bg-gradient-to-br", article.coverTone)} />
            <Image
                src={article.imageSrc}
                alt={article.imageAlt}
                fill
                priority={priority}
                sizes="(min-width: 1280px) 560px, (min-width: 1024px) calc(50vw - 96px), calc(100vw - 40px)"
                className="object-cover opacity-90 mix-blend-screen"
            />
            <div className="absolute inset-0 bg-[linear-gradient(180deg,rgba(5,16,24,0.02)_0%,rgba(5,16,24,0.36)_100%)]" />
        </div>
    );
}
