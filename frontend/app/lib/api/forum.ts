import { apiFetch } from "./client";

export interface ForumSpace {
    id: string;
    space_type: "global" | "annual" | "school_program";
    display_name: string;
    academic_year?: number;
    school_code?: string;
    program_code?: string;
    joined: boolean;
}

export interface ForumThread {
    id: string;
    space_id: string;
    title: string;
    created_at: string;
    updated_at: string;
}

export interface ReactionTally {
    emoji: string;
    count: number;
    mine: boolean;
}

export interface ForumPost {
    id: string;
    thread_id: string;
    body: string;
    quoted_experience_id?: string;
    created_at: string;
    reactions?: ReactionTally[];
}

export interface Page<T> {
    data: T[];
    next_cursor: string;
}

/** GET /api/v1/forum/spaces — public; `joined` reflects the caller when logged in. */
export function listSpaces() {
    return apiFetch<{ data: ForumSpace[] }>("/api/v1/forum/spaces");
}

/** POST /api/v1/forum/spaces/{id}/join (auth) */
export function joinSpace(spaceId: string) {
    return apiFetch<void>(`/api/v1/forum/spaces/${spaceId}/join`, { method: "POST" });
}

/** POST /api/v1/forum/spaces/{id}/leave (auth) */
export function leaveSpace(spaceId: string) {
    return apiFetch<void>(`/api/v1/forum/spaces/${spaceId}/leave`, { method: "POST" });
}

/** GET /api/v1/forum/spaces/{id}/threads — public, keyset paginated. */
export function listThreads(spaceId: string, cursor?: string) {
    return apiFetch<Page<ForumThread>>(`/api/v1/forum/spaces/${spaceId}/threads`, {
        query: { limit: 20, cursor }
    });
}

/** POST /api/v1/forum/spaces/{id}/threads (auth) — creates a thread + its first post. */
export function createThread(spaceId: string, input: { title: string; body: string }) {
    return apiFetch<{ thread: ForumThread; first_post: ForumPost }>(
        `/api/v1/forum/spaces/${spaceId}/threads`,
        { method: "POST", body: input }
    );
}

/** GET /api/v1/forum/threads/{id}/posts — public, keyset paginated. */
export function listPosts(threadId: string, cursor?: string) {
    return apiFetch<Page<ForumPost>>(`/api/v1/forum/threads/${threadId}/posts`, {
        query: { limit: 20, cursor }
    });
}

/** POST /api/v1/forum/threads/{id}/posts (auth) */
export function createPost(threadId: string, body: string) {
    return apiFetch<{ data: ForumPost }>(`/api/v1/forum/threads/${threadId}/posts`, {
        method: "POST",
        body: { body }
    });
}

/** PUT /api/v1/forum/posts/{postID}/reactions/{emoji} (auth) */
export function addPostReaction(postID: string, emoji: string) {
    return apiFetch<void>(
        `/api/v1/forum/posts/${encodeURIComponent(postID)}/reactions/${encodeURIComponent(emoji)}`,
        { method: "PUT" }
    );
}

/** DELETE /api/v1/forum/posts/{postID}/reactions/{emoji} (auth) */
export function removePostReaction(postID: string, emoji: string) {
    return apiFetch<void>(
        `/api/v1/forum/posts/${encodeURIComponent(postID)}/reactions/${encodeURIComponent(emoji)}`,
        { method: "DELETE" }
    );
}
