import { apiFetch } from "./client";

export interface SchoolHit {
    id: string;
    school_code: string;
    school_name: string;
    institution_type: string;
    is_active: boolean;
}

export interface ProgramHit {
    id: string;
    program_identifier: string;
    academic_year: number;
    school_code: string;
    school_name: string;
    admission_program_name: string;
    special_talent_target: string;
}

export interface ExperienceHit {
    id: string;
    title: string;
    snippet: string;
}

export interface SearchResults {
    schools?: SchoolHit[];
    programs?: ProgramHit[];
    experiences?: ExperienceHit[];
}

export type SearchType = "schools" | "programs" | "experiences";

/** GET /api/v1/search — public, rate-limited to 30/min per caller. */
export function search(query: string, types?: SearchType[]) {
    return apiFetch<{ query: string; results: SearchResults }>("/api/v1/search", {
        query: { q: query, types: types?.join(",") }
    });
}
