import type { TaggedExperience } from "../../lib/api/content";

/** The formal API has no article bookmark endpoint yet. */
export default function SaveExperienceButton({ experience }: { experience: TaggedExperience }) {
    return (
        <p
            className="font-sans text-sm text-copy-muted"
            aria-label={`「${experience.title}」收藏功能尚未開放`}
        >
            文章收藏功能尚未開放
        </p>
    );
}
