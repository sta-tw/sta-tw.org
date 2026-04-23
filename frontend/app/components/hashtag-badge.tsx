type HashtagBadgeProps = {
    tag: string;
};

export default function HashtagBadge({ tag }: HashtagBadgeProps) {
    return (
        <span className="inline-flex rounded-full bg-accent-green/55 px-3 py-1 text-sm font-semibold text-ink">
            #{tag}
        </span>
    );
}
