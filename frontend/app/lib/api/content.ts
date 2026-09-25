import { apiFetch } from "./client";

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

/** GET /api/v1/experiences — public, keyset paginated. */
export function listExperiences(
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

/** GET /api/v1/experiences/{experienceID} — public published experience. */
export function getExperience(experienceID: string, options?: { signal?: AbortSignal }) {
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
