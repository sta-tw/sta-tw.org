import { apiFetch } from "./client";
import { ApiError } from "./types";

export interface ReactionTally {
    emoji: string;
    count: number;
    mine: boolean;
}

export interface Experience {
    id: string;
    author_type: string;
    admission_outcome: string;
    visibility: "hidden" | "published" | "unpublished";
    current_revision_id: string;
    title: string;
    body: string;
    revision_number: number;
    created_at: string;
    updated_at: string;
    tags?: string[];
    reactions?: ReactionTally[];
}

export interface ExperiencePage {
    data: Experience[];
    next_cursor: string;
}

export interface ExperienceInput {
    title: string;
    body: string;
    author_type: string;
    admission_outcome: string;
    tags: string[];
}

/** Rough reading time at about 300 CJK characters a minute. */
export function estimateReadingMinutes(body: string) {
    return Math.max(1, Math.ceil(Array.from(body.trim()).length / 300));
}

export const EXPERIENCE_TAG_LIMIT = 8;
export const EXPERIENCE_TITLE_MAX_LENGTH = 50;
export const EXPERIENCE_SUMMARY_MAX_LENGTH = 500;

export function normalizeTags(tags: string[]) {
    return Array.from(
        new Set(tags.map((value) => value.trim().replace(/^#+/, "").toLowerCase()).filter(Boolean))
    ).slice(0, EXPERIENCE_TAG_LIMIT);
}

export type ExperienceReviewStatus = "draft" | "pending_review" | "approved" | "rejected";
export type ExperienceThumbnail =
    | { source: "body" }
    | { source: "upload"; url: string }
    | { source: "none" };

export interface ExperienceExtras {
    tags: string[];
    /** Derived from the saved body; the API has no separate summary field. */
    summary: string;
    /** Derived from the first Markdown image; uploaded thumbnails are unsupported. */
    thumbnail: ExperienceThumbnail;
}

export type TaggedExperience = Experience & ExperienceExtras;
export type MyExperience = TaggedExperience & { review_status: ExperienceReviewStatus };

/** First Markdown image URL in `body` (http/https only). */
export function firstBodyImage(body: string) {
    return body.match(/!\[[^\]]*\]\((https?:\/\/[^\s)]+)/)?.[1];
}

export function experienceThumbnailUrl(experience: Pick<TaggedExperience, "body" | "thumbnail">) {
    const { thumbnail } = experience;
    return thumbnail.source === "body" ? firstBodyImage(experience.body) : undefined;
}

/** Plain-text start of a Markdown body for cards. */
export function excerptOf(body: string, length = 140) {
    const text = body
        .replace(/!\[[^\]]*\]\([^)]*\)/g, "")
        .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
        .replace(/^[#>\-*\s]+/gm, "")
        .replace(/[*_`]/g, "")
        .replace(/\s+/g, " ")
        .trim();
    return text.length > length ? `${text.slice(0, length)}…` : text;
}

export const experienceExcerpt = (experience: Pick<TaggedExperience, "body" | "summary">) =>
    experience.summary.trim() || excerptOf(experience.body);

function withExtras<T extends Experience>(experience: T): T & ExperienceExtras {
    const image = firstBodyImage(experience.body);
    return {
        ...experience,
        tags: experience.tags ?? [],
        summary: excerptOf(experience.body),
        thumbnail: image ? { source: "body" } : { source: "none" }
    };
}

/** GET /api/v1/experiences — public, keyset paginated. */
export async function listExperiences(
    options: { limit?: number; cursor?: string; signal?: AbortSignal } = {}
) {
    const page = await apiFetch<ExperiencePage>("/api/v1/experiences", {
        query: { limit: options.limit, cursor: options.cursor },
        signal: options.signal
    });
    return { ...page, data: page.data.map(withExtras) };
}

/** GET /api/v1/experiences/{experienceID} — public published experience. */
export async function getExperience(experienceID: string, options?: { signal?: AbortSignal }) {
    const result = await apiFetch<{ data: Experience }>(
        `/api/v1/experiences/${encodeURIComponent(experienceID)}`,
        { signal: options?.signal }
    );
    return { data: withExtras(result.data) };
}

/** POST /api/v1/experiences — creates a hidden draft. */
export async function createExperience(input: ExperienceInput) {
    const result = await apiFetch<{ data: Experience }>("/api/v1/experiences", {
        method: "POST",
        body: { ...input, tags: normalizeTags(input.tags) }
    });
    return { data: withExtras(result.data) };
}

/** POST /api/v1/experiences/{id}/revisions — creates a reviewable revision. */
export async function createExperienceRevision(experienceID: string, input: ExperienceInput) {
    const result = await apiFetch<{ data: Experience }>(
        `/api/v1/experiences/${encodeURIComponent(experienceID)}/revisions`,
        { method: "POST", body: { ...input, tags: normalizeTags(input.tags) } }
    );
    return { data: withExtras(result.data) };
}

/** POST /api/v1/experience-revisions/{revisionID}/submit. */
export function submitExperienceRevision(revisionID: string) {
    return apiFetch<{ data: Experience }>(
        `/api/v1/experience-revisions/${encodeURIComponent(revisionID)}/submit`,
        { method: "POST" }
    );
}

/** POST /api/v1/experiences/{id}/unpublish. */
export function unpublishExperience(experienceID: string) {
    return apiFetch<void>(`/api/v1/experiences/${encodeURIComponent(experienceID)}/unpublish`, {
        method: "POST"
    });
}

/** PUT /api/v1/experiences/{experienceID}/reactions/{emoji}. */
export function addExperienceReaction(experienceID: string, emoji: string) {
    return apiFetch<void>(
        `/api/v1/experiences/${encodeURIComponent(experienceID)}/reactions/${encodeURIComponent(emoji)}`,
        { method: "PUT" }
    );
}

/** DELETE /api/v1/experiences/{experienceID}/reactions/{emoji}. */
export function removeExperienceReaction(experienceID: string, emoji: string) {
    return apiFetch<void>(
        `/api/v1/experiences/${encodeURIComponent(experienceID)}/reactions/${encodeURIComponent(emoji)}`,
        { method: "DELETE" }
    );
}

/** GET /api/v1/experiences/tags — public tag usage counts. */
export function listExperienceTags(options?: { signal?: AbortSignal }) {
    return apiFetch<{ data: { tag: string; count: number }[] }>("/api/v1/experiences/tags", {
        signal: options?.signal
    });
}

/** GET /api/v1/me/experiences — caller's drafts and published content. */
export async function listMyExperiences(options?: { signal?: AbortSignal }) {
    const result = await apiFetch<{ data: MyExperience[] }>("/api/v1/me/experiences", {
        signal: options?.signal
    });
    return { data: result.data.map(withExtras) };
}

/** The supported API returns the caller's list, so resolve an item from it. */
export async function getMyExperience(experienceID: string, options?: { signal?: AbortSignal }) {
    const { data } = await listMyExperiences(options);
    const experience = data.find((item) => item.id === experienceID);
    if (!experience) throw new ApiError(404, "not_found", "找不到這篇文章。");
    return { data: experience };
}

/** GET /api/v1/users/{username}/experiences — published only. */
export async function listUserExperiences(username: string, options?: { signal?: AbortSignal }) {
    const result = await apiFetch<{ data: Experience[] }>(
        `/api/v1/users/${encodeURIComponent(username)}/experiences`,
        { signal: options?.signal }
    );
    return { data: result.data.map(withExtras) };
}

function unsupportedBookmark() {
    return new ApiError(501, "not_supported", "正式後端目前尚未提供文章收藏功能。");
}

export function listSavedExperiences(_options?: {
    signal?: AbortSignal;
}): Promise<{ data: TaggedExperience[] }> {
    void _options;
    return Promise.reject(unsupportedBookmark());
}

export function getSavedExperienceStatus(
    _experienceID: string,
    _options?: { signal?: AbortSignal }
) {
    void _experienceID;
    void _options;
    return Promise.reject(unsupportedBookmark());
}

export function saveExperience(_experience: TaggedExperience): Promise<void> {
    void _experience;
    return Promise.reject(unsupportedBookmark());
}

export function unsaveExperience(_experienceID: string): Promise<void> {
    void _experienceID;
    return Promise.reject(unsupportedBookmark());
}
