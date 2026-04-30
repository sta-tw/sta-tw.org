import Image from "next/image";
import Link from "next/link";
import { Separator } from "radix-ui";
import { twMerge } from "tailwind-merge";
import type { ArticlePreview } from "../lib/article-list";
import HashtagBadge from "./hashtag-badge";

type ArticleCardProps = {
    article: ArticlePreview;
};

export default function ArticleCard({ article }: ArticleCardProps) {
    return (
        <article className="h-full">
            <Link
                href={`/articles/${article.id}`}
                className="group flex h-full cursor-pointer flex-col overflow-hidden rounded-[var(--radius-panel)] border border-ink/10 bg-white shadow-[var(--shadow-card)] transition-transform duration-300 hover:-translate-y-1 focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ink"
            >
                <div className="relative aspect-[16/10] overflow-hidden">
                    <div className={twMerge("absolute inset-0 bg-gradient-to-br", article.coverTone)} />
                    <Image
                        src={article.imageSrc}
                        alt={article.imageAlt}
                        fill
                        sizes="(min-width: 1280px) 544px, (min-width: 1024px) calc(50vw - 72px), calc(100vw - 40px)"
                        className="object-cover opacity-90 mix-blend-screen transition-transform duration-500 group-hover:scale-105"
                    />
                    <div className="absolute inset-0 bg-[linear-gradient(180deg,rgba(5,16,24,0.08)_0%,rgba(5,16,24,0.42)_100%)]" />
                    <div className="absolute top-5 left-5 inline-flex rounded-full bg-white/88 px-3 py-1 text-sm font-semibold text-ink shadow-sm">
                        {article.category}
                    </div>
                </div>

                <div className="flex flex-1 flex-col gap-5 p-5 sm:p-6">
                    <div className="flex flex-wrap items-center gap-3 text-sm font-medium text-copy-muted">
                        <span>{article.publishedAt}</span>
                        <Separator.Root
                            decorative
                            orientation="vertical"
                            className="h-4 w-px bg-ink/15"
                        />
                        <span>{article.readTime}</span>
                    </div>

                    <div className="space-y-3">
                        <h3 className="font-serif text-[1.75rem] leading-tight text-balance text-ink">
                            {article.title}
                        </h3>
                        <p className="font-sans text-base leading-7 font-medium text-copy-muted">
                            {article.excerpt}
                        </p>
                    </div>

                    <ul className="mt-auto flex flex-wrap gap-2 pt-2">
                        {article.tags.map((tag) => (
                            <li key={tag}>
                                <HashtagBadge tag={tag} />
                            </li>
                        ))}
                    </ul>
                </div>
            </Link>
        </article>
    );
}
