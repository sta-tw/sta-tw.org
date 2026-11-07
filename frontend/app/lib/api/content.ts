import { getCurrentAccount } from "./auth";
import { apiFetch } from "./client";
import { mockResponse, mockStore } from "./mock";

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

/** Experience plus the tags field the backend will add. */
export type TaggedExperience = Experience & { tags: string[] };

/** An item of GET /api/v1/me/experiences. */
export type MyExperience = TaggedExperience & { review_status: ExperienceReviewStatus };

// ---------------------------------------------------------------------------
// Not backed yet — see lib/api/mock.ts. Tags are kept per experience id;
// "my experiences" are snapshots of what this browser created/revised. The
// exported wrappers below add that bookkeeping around the real calls above.

const TAGS_KEY = "experience-tags";
const MINE_KEY = "my-experiences";

const readTagMap = () => mockStore.read<Record<string, string[]>>(TAGS_KEY, {});

/** A "my experiences" snapshot; review_status is set once submitted. */
type MyExperienceSnapshot = Experience & { review_status?: ExperienceReviewStatus };

const readMyExperiences = () => mockStore.read<MyExperienceSnapshot[]>(MINE_KEY, []);

// The tag list is shared by the article overview and the editor: one request
// until tags change (dropped on failure so a later call retries).
let tagsRequest: Promise<{ data: { tag: string; count: number }[] }> | null = null;

function rememberMyExperience(experience: MyExperienceSnapshot, tags?: string[]) {
    if (tags) {
        mockStore.write(TAGS_KEY, { ...readTagMap(), [experience.id]: normalizeTags(tags) });
        tagsRequest = null;
    }
    const others = readMyExperiences().filter((item) => item.id !== experience.id);
    mockStore.write(MINE_KEY, [experience, ...others]);
}

/** Adds `tags` to experiences. Takes the tag map once so a list maps
 * without re-reading storage per item. */
function withTags(tagMap: Record<string, string[]>) {
    return <T extends Experience>(experience: T): T & { tags: string[] } => ({
        ...experience,
        tags: tagMap[experience.id] ?? []
    });
}

/** GET /api/v1/experiences with the MOCK: `tags` field on every item. */
export async function listExperiences(
    options: { limit?: number; cursor?: string; signal?: AbortSignal } = {}
) {
    const page = await realListExperiences(options);
    return { data: page.data.map(withTags(readTagMap())), next_cursor: page.next_cursor };
}

/** GET /api/v1/experiences/{experienceID} with the MOCK: `tags` field. */
export async function getExperience(experienceID: string, options?: { signal?: AbortSignal }) {
    const { data } = await realGetExperience(experienceID, options);
    return { data: withTags(readTagMap())(data) };
}

/** POST /api/v1/experiences with MOCK: `tags` in the body (stored locally)
 * and the draft recorded in "my experiences". */
export async function createExperience(input: ExperienceInput & { tags: string[] }) {
    const { tags, ...body } = input;
    const result = await realCreateExperience(body);
    rememberMyExperience(result.data, tags);
    return result;
}

/** POST /api/v1/experiences/{id}/revisions with MOCK: `tags` in the body
 * (stored locally) and the revision recorded in "my experiences". */
export async function createExperienceRevision(
    experienceID: string,
    input: ExperienceInput & { tags: string[] }
) {
    const { tags, ...body } = input;
    const result = await realCreateExperienceRevision(experienceID, body);
    rememberMyExperience(result.data, tags);
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
        const tag = withTags(readTagMap());
        return {
            data: readMyExperiences().map((item) => ({
                ...tag(item),
                review_status: item.review_status ?? "draft"
            }))
        };
    }, options?.signal);
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
                      .map(withTags(readTagMap()))
                : []
        };
    }, options?.signal);
}
