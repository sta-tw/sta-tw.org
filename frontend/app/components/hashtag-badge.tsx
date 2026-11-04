import Link from "next/link";

type HashtagBadgeProps = {
    tag: string;
    /** Renders the badge as a link (e.g. to the tag-filtered overview). Leave
     * unset inside another link such as a ContentCard. */
    href?: string;
};

const badgeClass =
    "inline-flex rounded-full bg-accent-green/55 px-3 py-1 text-sm font-semibold text-ink";

export default function HashtagBadge({ tag, href }: HashtagBadgeProps) {
    if (href) {
        return (
            <Link
                href={href}
                className={`${badgeClass} focus-ring transition-opacity hover:opacity-80`}
            >
                #{tag}
            </Link>
        );
    }
    return <span className={badgeClass}>#{tag}</span>;
}
