import Link from "next/link";
import { useId, type ComponentPropsWithoutRef, type ReactNode } from "react";
import { twMerge } from "tailwind-merge";

type CardProps = Omit<ComponentPropsWithoutRef<"section">, "title"> & {
    /** Renders the panel heading (h2) and labels the section with it. */
    title?: ReactNode;
    description?: ReactNode;
    /** Shown beside the heading, e.g. a "mark all read" button. */
    actions?: ReactNode;
};

/** The site's panel surface. Use for grouped form sections and static
 * content blocks; use ContentCard for anything that links somewhere. */
export function Card({ className, title, description, actions, children, ...props }: CardProps) {
    const titleId = useId();
    return (
        <section
            aria-labelledby={title ? titleId : undefined}
            className={twMerge(
                "rounded-[var(--radius-panel)] border border-ink/10 bg-surface p-5 sm:p-7",
                className
            )}
            {...props}
        >
            {title && (
                <header className="flex flex-wrap items-center justify-between gap-3">
                    <div className="min-w-0">
                        <h2
                            id={titleId}
                            className="font-sans text-xl font-medium break-words text-ink sm:text-2xl"
                        >
                            {title}
                        </h2>
                        {description && (
                            <p className="mt-1 font-sans text-sm text-copy-muted sm:text-base">
                                {description}
                            </p>
                        )}
                    </div>
                    {actions}
                </header>
            )}
            {children}
        </section>
    );
}

const thumbnailPalettes = [
    ["bg-accent-green", "bg-accent-yellow"],
    ["bg-accent-yellow", "bg-accent-green-strong"],
    ["bg-accent-yellow-light", "bg-accent-yellow-strong"],
    ["bg-accent-green-strong", "bg-accent-yellow-light"]
] as const;

function hashSeed(seed: string) {
    let hash = 0;
    for (const char of seed) hash = (hash * 31 + char.charCodeAt(0)) | 0;
    return Math.abs(hash);
}

/** Decorative stand-in for content without its own image: the hero's
 * circle motif in the accent palette, picked deterministically from `seed`
 * so a given item always looks the same. */
function CardThumbnail({ seed, className }: { seed: string; className?: string }) {
    const hash = hashSeed(seed);
    const [base, circle] = thumbnailPalettes[hash % thumbnailPalettes.length];
    const position = ["-right-8 -bottom-10", "-top-10 -left-6", "-right-6 -top-12"][hash % 3];
    return (
        <div aria-hidden className={twMerge("relative overflow-hidden", base, className)}>
            <span className={twMerge("absolute size-40 rounded-full", circle, position)} />
            <span className="article-dots absolute inset-0 opacity-40 mix-blend-multiply" />
        </div>
    );
}

type ContentCardProps = {
    href: string;
    title: ReactNode;
    excerpt?: ReactNode;
    /** Small line above the title, e.g. a site name or category. */
    eyebrow?: ReactNode;
    /** Footer row, e.g. date / reading time / tags. */
    meta?: ReactNode;
    /** Opens in a new tab with rel="noopener noreferrer". */
    external?: boolean;
    className?: string;
} & (
    | { media: ReactNode; thumbnailSeed?: never }
    /** Stable id (e.g. article id) seeding the decorative CardThumbnail. */
    | { media?: never; thumbnailSeed: string }
);

/** Linked list card with a media column, used for articles, saved programs
 * and external links so they all share one hover/focus treatment. */
export function ContentCard({
    href,
    title,
    excerpt,
    eyebrow,
    meta,
    media,
    thumbnailSeed,
    external,
    className
}: ContentCardProps) {
    const linkProps = external ? { target: "_blank", rel: "noopener noreferrer" } : {};
    return (
        <Link
            href={href}
            {...linkProps}
            className={twMerge(
                "group grid overflow-hidden rounded-[var(--radius-panel)] border border-ink/10 bg-surface/80 focus-ring transition-[border-color,box-shadow,transform] hover:-translate-y-0.5 hover:border-ink/25 hover:shadow-[var(--shadow-card)] sm:grid-cols-[14rem_minmax(0,1fr)]",
                className
            )}
        >
            {media ?? <CardThumbnail seed={thumbnailSeed!} className="min-h-36 sm:min-h-full" />}
            <div className="flex min-w-0 flex-col p-5 sm:p-6">
                {eyebrow && <p className="font-sans text-sm text-copy-muted">{eyebrow}</p>}
                <h3 className="font-sans text-xl leading-snug font-medium text-ink sm:text-2xl">
                    {title}
                </h3>
                {excerpt && (
                    <p className="mt-3 line-clamp-3 font-sans text-base leading-7 text-copy-muted">
                        {excerpt}
                    </p>
                )}
                {meta && (
                    <div className="mt-auto flex flex-wrap items-center gap-x-4 gap-y-2 pt-4 font-sans text-sm text-copy-muted">
                        {meta}
                    </div>
                )}
            </div>
        </Link>
    );
}
