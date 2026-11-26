import { getCurrentAccount } from "./auth";
import { apiFetch } from "./client";
import { mockResponse, mockStore } from "./mock";
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
    reactions?: ReactionTally[];
}

export interface ExperiencePage {
    data: Experience[];
    next_cursor: string;
}

/** GET /api/v1/experiences — public, keyset paginated. Wrapped below. */
function realListExperiences(
    options: { limit?: number; cursor?: string; signal?: AbortSignal } = {}
) {
    return apiFetch<ExperiencePage>("/api/v1/experiences", {
        query: {
            limit: options.limit,
            cursor: options.cursor
        },
        signal: options.signal
    });
}

/** GET /api/v1/experiences/{experienceID} — public published experience.
 * Wrapped below. */
function realGetExperience(experienceID: string, options?: { signal?: AbortSignal }) {
    return apiFetch<{ data: Experience }>(
        `/api/v1/experiences/${encodeURIComponent(experienceID)}`,
        { signal: options?.signal }
    );
}

/** PUT /api/v1/experiences/{experienceID}/reactions/{emoji} */
export function addExperienceReaction(experienceID: string, emoji: string) {
    return apiFetch<void>(
        `/api/v1/experiences/${encodeURIComponent(experienceID)}/reactions/${encodeURIComponent(emoji)}`,
        { method: "PUT" }
    );
}

/** DELETE /api/v1/experiences/{experienceID}/reactions/{emoji} */
export function removeExperienceReaction(experienceID: string, emoji: string) {
    return apiFetch<void>(
        `/api/v1/experiences/${encodeURIComponent(experienceID)}/reactions/${encodeURIComponent(emoji)}`,
        { method: "DELETE" }
    );
}

export interface ExperienceInput {
    title: string;
    body: string;
    /** Free-form today (backend stores text, default "-"), e.g. "116 特選生". */
    author_type: string;
    /** Free-form, e.g. "正取", "備取", "未錄取". */
    admission_outcome: string;
}

/** Rough reading time at ~300 CJK characters a minute. */
export function estimateReadingMinutes(body: string) {
    return Math.max(1, Math.ceil(Array.from(body.trim()).length / 300));
}

/** POST /api/v1/experiences — creates a hidden draft (revision 1). Wrapped
 * below. */
function realCreateExperience(input: ExperienceInput) {
    return apiFetch<{ data: Experience }>("/api/v1/experiences", { method: "POST", body: input });
}

/** POST /api/v1/experiences/{id}/revisions — a new revision to review.
 * Wrapped below. */
function realCreateExperienceRevision(experienceID: string, input: ExperienceInput) {
    return apiFetch<{ data: Experience }>(
        `/api/v1/experiences/${encodeURIComponent(experienceID)}/revisions`,
        { method: "POST", body: input }
    );
}

/** POST /api/v1/experience-revisions/{revisionID}/submit — sends a revision
 * to admin review. Wrapped below. */
function realSubmitExperienceRevision(revisionID: string) {
    return apiFetch<{ data: Experience }>(
        `/api/v1/experience-revisions/${encodeURIComponent(revisionID)}/submit`,
        { method: "POST" }
    );
}

/** POST /api/v1/experiences/{id}/unpublish. Wrapped below. */
function realUnpublishExperience(experienceID: string) {
    return apiFetch<void>(`/api/v1/experiences/${encodeURIComponent(experienceID)}/unpublish`, {
        method: "POST"
    });
}

export const EXPERIENCE_TAG_LIMIT = 8;

/** Lower-cases ASCII, strips a leading "#", trims and de-duplicates. */
export function normalizeTags(tags: string[]) {
    const seen = new Set<string>();
    for (const raw of tags) {
        const tag = raw.trim().replace(/^#+/, "").toLowerCase();
        if (tag) seen.add(tag);
    }
    return Array.from(seen).slice(0, EXPERIENCE_TAG_LIMIT);
}

/** experience_revisions.review_status of the caller's latest revision. */
export type ExperienceReviewStatus = "draft" | "pending_review" | "approved" | "rejected";

/** How an article card gets its picture: the first image in the body, an
 * uploaded one, or none (cards then fall back to a generated pattern). */
export type ExperienceThumbnail =
    | { source: "body" }
    | { source: "upload"; url: string }
    | { source: "none" };

/** Fields the article editor sends beside ExperienceInput. */
export interface ExperienceExtras {
    tags: string[];
    /** Card / search excerpt; blank falls back to the start of the body. */
    summary: string;
    thumbnail: ExperienceThumbnail;
}

/** Experience plus the fields the backend will add (tags, summary,
 * thumbnail). */
export type TaggedExperience = Experience & ExperienceExtras;

export const EXPERIENCE_TITLE_MAX_LENGTH = 50;
export const EXPERIENCE_SUMMARY_MAX_LENGTH = 500;

/** First Markdown image URL in `body` (http/https only). */
export function firstBodyImage(body: string) {
    return body.match(/!\[[^\]]*\]\((https?:\/\/[^\s)]+)/)?.[1];
}

/** The picture an article card should show, if any. */
export function experienceThumbnailUrl(experience: Pick<TaggedExperience, "body" | "thumbnail">) {
    const { thumbnail } = experience;
    if (thumbnail.source === "upload") return thumbnail.url;
    if (thumbnail.source === "body") return firstBodyImage(experience.body);
    return undefined;
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

/** The author's summary, or the start of the body without one. */
export const experienceExcerpt = (experience: Pick<TaggedExperience, "body" | "summary">) =>
    experience.summary.trim() || excerptOf(experience.body);

/** An item of GET /api/v1/me/experiences. */
export type MyExperience = TaggedExperience & { review_status: ExperienceReviewStatus };

// ---------------------------------------------------------------------------
// Not backed yet — see lib/api/mock.ts. Tags, summary and thumbnail are kept
// per experience id; "my experiences" are snapshots of what this browser
// created/revised. The exported wrappers below add that bookkeeping around
// the real calls above.

const TAGS_KEY = "experience-tags";
const EXTRAS_KEY = "experience-extras";
const MINE_KEY = "my-experiences";
const SAVED_KEY = "saved-experiences";

type StoredExtras = Pick<ExperienceExtras, "summary" | "thumbnail">;

const readTagMap = () => mockStore.read<Record<string, string[]>>(TAGS_KEY, {});
const readExtrasMap = () => mockStore.read<Record<string, StoredExtras>>(EXTRAS_KEY, {});

/** A "my experiences" snapshot; review_status is set once submitted. */
type MyExperienceSnapshot = Experience & { review_status?: ExperienceReviewStatus };

const readMyExperiences = () => mockStore.read<MyExperienceSnapshot[]>(MINE_KEY, []);

// The tag list is shared by the article overview and the editor: one request
// until tags change (dropped on failure so a later call retries).
let tagsRequest: Promise<{ data: { tag: string; count: number }[] }> | null = null;

function rememberMyExperience(
    experience: MyExperienceSnapshot,
    extras?: Omit<ExperienceExtras, "tags"> & { tags: string[] }
) {
    if (extras) {
        mockStore.write(TAGS_KEY, {
            ...readTagMap(),
            [experience.id]: normalizeTags(extras.tags)
        });
        mockStore.write(EXTRAS_KEY, {
            ...readExtrasMap(),
            [experience.id]: { summary: extras.summary.trim(), thumbnail: extras.thumbnail }
        });
        tagsRequest = null;
    }
    const others = readMyExperiences().filter((item) => item.id !== experience.id);
    mockStore.write(MINE_KEY, [experience, ...others]);
}

const noExtras: StoredExtras = { summary: "", thumbnail: { source: "body" } };

/** Adds the mocked fields to experiences. Reads storage once so a list maps
 * without re-reading per item. */
function withExtras() {
    const tagMap = readTagMap();
    const extrasMap = readExtrasMap();
    return <T extends Experience>(experience: T): T & ExperienceExtras => ({
        ...experience,
        ...(extrasMap[experience.id] ?? noExtras),
        tags: tagMap[experience.id] ?? []
    });
}

/** GET /api/v1/experiences with the MOCK: tags / summary / thumbnail
 * fields on every item. */
export async function listExperiences(
    options: { limit?: number; cursor?: string; signal?: AbortSignal } = {}
) {
    const page = await realListExperiences(options);
    return { data: page.data.map(withExtras()), next_cursor: page.next_cursor };
}

/** GET /api/v1/experiences/{experienceID} with the MOCK: fields. */
export async function getExperience(experienceID: string, options?: { signal?: AbortSignal }) {
    const { data } = await realGetExperience(experienceID, options);
    return { data: withExtras()(data) };
}

/** POST /api/v1/experiences with MOCK: tags / summary / thumbnail in the
 * body (stored locally) and the draft recorded in "my experiences". */
export async function createExperience(input: ExperienceInput & ExperienceExtras) {
    const { tags, summary, thumbnail, ...body } = input;
    const result = await realCreateExperience(body);
    rememberMyExperience(result.data, { tags, summary, thumbnail });
    return result;
}

/** POST /api/v1/experiences/{id}/revisions with MOCK: tags / summary /
 * thumbnail in the body (stored locally) and the revision recorded in "my
 * experiences". */
export async function createExperienceRevision(
    experienceID: string,
    input: ExperienceInput & ExperienceExtras
) {
    const { tags, summary, thumbnail, ...body } = input;
    const result = await realCreateExperienceRevision(experienceID, body);
    rememberMyExperience(result.data, { tags, summary, thumbnail });
    return result;
}

/** POST /api/v1/experience-revisions/{revisionID}/submit; MOCK: records the
 * pending_review status in "my experiences". */
export async function submitExperienceRevision(revisionID: string) {
    const result = await realSubmitExperienceRevision(revisionID);
    rememberMyExperience({ ...result.data, review_status: "pending_review" });
    return result;
}

/** POST /api/v1/experiences/{id}/unpublish; MOCK: marks the snapshot in "my
 * experiences" unpublished. */
export async function unpublishExperience(experienceID: string) {
    await realUnpublishExperience(experienceID);
    const target = readMyExperiences().find((item) => item.id === experienceID);
    if (target) rememberMyExperience({ ...target, visibility: "unpublished" });
}

/** Longest edge of an uploaded thumbnail. */
const THUMBNAIL_MAX_EDGE = 960;

/** MOCK: POST /api/v1/experiences/thumbnails — multipart `file`, returns
 * the hosted image URL. The mock scales the image down and returns it as a
 * data URL so it fits in local storage. */
export function uploadExperienceThumbnail(file: File) {
    return mockResponse(async () => {
        const bitmap = await createImageBitmap(file);
        try {
            const scale = Math.min(1, THUMBNAIL_MAX_EDGE / Math.max(bitmap.width, bitmap.height));
            const canvas = document.createElement("canvas");
            canvas.width = Math.round(bitmap.width * scale);
            canvas.height = Math.round(bitmap.height * scale);
            canvas.getContext("2d")?.drawImage(bitmap, 0, 0, canvas.width, canvas.height);
            return { data: { url: canvas.toDataURL("image/jpeg", 0.8) } };
        } finally {
            bitmap.close();
        }
    });
}

/** MOCK: GET /api/v1/experiences/tags — tag usage counts, most used first. */
export function listExperienceTags() {
    tagsRequest ??= mockResponse(() => {
        const counts = new Map<string, number>();
        for (const tags of Object.values(readTagMap())) {
            for (const tag of tags) counts.set(tag, (counts.get(tag) ?? 0) + 1);
        }
        const data = Array.from(counts, ([tag, count]) => ({ tag, count })).sort(
            (a, b) => b.count - a.count
        );
        return { data };
    }).catch((cause: unknown) => {
        tagsRequest = null;
        throw cause;
    });
    return tagsRequest;
}

/** MOCK: GET /api/v1/me/experiences — the caller's experiences in every
 * visibility, most recently changed first, with the latest revision's
 * `review_status` (this browser only knows draft vs. submitted). */
export function listMyExperiences(options?: { signal?: AbortSignal }) {
    return mockResponse((): { data: MyExperience[] } => {
        const extras = withExtras();
        return {
            data: readMyExperiences().map((item) => ({
                ...extras(item),
                review_status: item.review_status ?? "draft"
            }))
        };
    }, options?.signal);
}

/** MOCK: GET /api/v1/me/experiences/{experienceID} — one of the caller's
 * experiences at its latest revision, for the editor. */
export function getMyExperience(experienceID: string, options?: { signal?: AbortSignal }) {
    return listMyExperiences(options).then(({ data }) => {
        const experience = data.find((item) => item.id === experienceID);
        if (!experience) throw new ApiError(404, "not_found", "找不到這篇文章。");
        return { data: experience };
    });
}

/** MOCK: GET /api/v1/users/{username}/experiences — published only. The
 * mock only knows this browser's own experiences, so anyone other than the
 * signed-in user gets an empty list. */
export function listUserExperiences(username: string, options?: { signal?: AbortSignal }) {
    return mockResponse(async () => {
        const me = await getCurrentAccount().catch(() => null);
        const isSelf = me?.account.username === username;
        return {
            data: isSelf
                ? readMyExperiences()
                      .filter((item) => item.visibility === "published")
                      .map(withExtras())
                : []
        };
    }, options?.signal);
}

/** A bookmarked article; the mock keeps a snapshot for the list. */
type SavedExperience = { saved_at: string; experience: TaggedExperience };

const readSaved = () => mockStore.read<SavedExperience[]>(SAVED_KEY, []);

/** MOCK: GET /api/v1/saved-experiences — newest first. */
export function listSavedExperiences(options?: { signal?: AbortSignal }) {
    return mockResponse(
        () => ({ data: readSaved().map((item) => item.experience) }),
        options?.signal
    );
}

/** MOCK: GET /api/v1/saved-experiences/{experienceID} */
export function getSavedExperienceStatus(experienceID: string, options?: { signal?: AbortSignal }) {
    return mockResponse(
        () => ({
            data: { saved: readSaved().some((item) => item.experience.id === experienceID) }
        }),
        options?.signal
    );
}

/** MOCK: PUT /api/v1/saved-experiences/{experienceID} — idempotent. */
export function saveExperience(experience: TaggedExperience) {
    return mockResponse(() => {
        const others = readSaved().filter((item) => item.experience.id !== experience.id);
        mockStore.write(SAVED_KEY, [{ saved_at: new Date().toISOString(), experience }, ...others]);
    });
}

/** MOCK: DELETE /api/v1/saved-experiences/{experienceID} — idempotent. */
export function unsaveExperience(experienceID: string) {
    return mockResponse(() => {
        mockStore.write(
            SAVED_KEY,
            readSaved().filter((item) => item.experience.id !== experienceID)
        );
    });
}
