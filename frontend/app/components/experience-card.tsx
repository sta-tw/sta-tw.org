import type { ReactNode } from "react";
import { readingTimeOf } from "../article/article-format";
import {
    experienceExcerpt,
    experienceThumbnailUrl,
    type TaggedExperience
} from "../lib/api/content";
import { formatNumericDate } from "../lib/format";
import { experienceHref } from "../lib/routes";
import { ContentCard } from "./card";
import HashtagBadge from "./hashtag-badge";

type CardExperience = Pick<
    TaggedExperience,
    "id" | "title" | "body" | "summary" | "thumbnail" | "tags" | "updated_at"
>;

/** An experience article as a ContentCard: its thumbnail (or the generated
 * pattern), summary, date, reading time and tags. Links to the article
 * unless `href` is null (previews, unpublished drafts). */
export default function ExperienceCard({
    experience,
    href,
    eyebrow,
    tagLimit = 3,
    className
}: {
    experience: CardExperience;
    href?: string | null;
    eyebrow?: ReactNode;
    tagLimit?: number;
    className?: string;
}) {
    const thumbnail = experienceThumbnailUrl(experience);
    const media = thumbnail && (
        // Thumbnails are user-supplied URLs of unknown size, so a plain img
        // (next/image needs known remote hosts) cropped to the media column.
        // eslint-disable-next-line @next/next/no-img-element
        <img
            src={thumbnail}
            alt=""
            loading="lazy"
            className="aspect-video size-full object-cover sm:aspect-auto sm:min-h-36"
        />
    );
    const shared = {
        href: href === null ? undefined : (href ?? experienceHref(experience.id)),
        title: experience.title || "（未命名文章）",
        excerpt: experienceExcerpt(experience),
        eyebrow,
        className,
        meta: (
            <>
                <time dateTime={experience.updated_at}>
                    {formatNumericDate(experience.updated_at)}
                </time>
                <span>閱讀時間: {readingTimeOf(experience.body)}</span>
                {experience.tags.slice(0, tagLimit).map((tag) => (
                    <HashtagBadge key={tag} tag={tag} />
                ))}
            </>
        )
    };
    return media ? (
        <ContentCard {...shared} media={media} />
    ) : (
        <ContentCard {...shared} thumbnailSeed={experience.id} />
    );
}
