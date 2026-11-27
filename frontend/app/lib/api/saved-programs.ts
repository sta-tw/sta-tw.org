import { apiFetch } from "./client";

/** A program the user marked "我對此學系有興趣". */
export interface SavedProgram {
    program_identifier: string;
    academic_year: number;
    school_name: string;
    admission_program_name: string;
    saved_at: string;
}

/** GET /api/v1/saved-programs — newest first. */
export function listSavedPrograms(options?: { signal?: AbortSignal }) {
    return apiFetch<{ data: SavedProgram[] }>("/api/v1/saved-programs", {
        signal: options?.signal
    });
}

/** GET /api/v1/saved-programs/{identifier} */
export function getSavedProgramStatus(identifier: string, options?: { signal?: AbortSignal }) {
    return apiFetch<{ data: { saved: boolean } }>(
        `/api/v1/saved-programs/${encodeURIComponent(identifier)}`,
        { signal: options?.signal }
    );
}

/** PUT /api/v1/saved-programs/{identifier} — idempotent. */
export function saveProgram(identifier: string) {
    return apiFetch<{ data: SavedProgram }>(
        `/api/v1/saved-programs/${encodeURIComponent(identifier)}`,
        { method: "PUT" }
    );
}

/** DELETE /api/v1/saved-programs/{identifier} — idempotent. */
export function unsaveProgram(identifier: string) {
    return apiFetch<void>(`/api/v1/saved-programs/${encodeURIComponent(identifier)}`, {
        method: "DELETE"
    });
}
