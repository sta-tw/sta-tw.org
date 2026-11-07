import type { ReactNode } from "react";
import { twMerge } from "tailwind-merge";
import HashtagBadge from "../components/hashtag-badge";
import { formatNumericDate } from "../lib/format";
import { articleOverviewHref } from "../lib/routes";

type ArticleHeaderProps = {
    title: string;
    /** e.g. "3 min" */
    readTime: string;
    /** ISO date or timestamp of the last update. */
    date: string;
    /** Extra meta entries shown before the reading time. */
    details?: ReactNode;
    /** Rendered as hashtag links to the filtered article overview. */
    tags?: string[];
    className?: string;
};

/** Centered title + meta row shared by static and experience articles. */
export default function ArticleHeader({
    title,
    readTime,
    date,
    details,
    tags,
    className
}: ArticleHeaderProps) {
    return (
        <header className={twMerge("border-b border-ink/10 pb-8 text-center sm:pb-10", className)}>
            <h1 className="font-sans text-3xl leading-tight font-medium tracking-[-0.03em] text-ink sm:text-4xl lg:text-5xl">
                {title}
            </h1>
            <div className="mt-5 flex flex-wrap items-center justify-center gap-x-5 gap-y-2 font-sans text-sm text-copy-muted sm:text-base">
                {details}
                <span>閱讀時間: {readTime}</span>
                <time dateTime={date}>Update Date : {formatNumericDate(date)}</time>
            </div>
            {tags && tags.length > 0 && (
                <ul aria-label="標籤" className="mt-5 flex flex-wrap justify-center gap-2">
                    {tags.map((tag) => (
                        <li key={tag}>
                            <HashtagBadge tag={tag} href={articleOverviewHref({ tag })} />
                        </li>
                    ))}
                </ul>
            )}
        </header>
    );
}
