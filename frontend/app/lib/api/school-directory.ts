import { apiFetch } from "./client";

export type AccountSchoolType = "university" | "senior_high";

export interface AccountSchoolDirectoryEntry {
    id: string;
    name: string;
    school_type: AccountSchoolType;
    is_active: boolean;
    updated_at: string;
}

/** GET /api/v1/account-schools — active schools used by account profiles. */
export function listAccountSchoolDirectory(options?: { signal?: AbortSignal }) {
    return apiFetch<{ data: AccountSchoolDirectoryEntry[] }>("/api/v1/account-schools", {
        signal: options?.signal
    });
}
