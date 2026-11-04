import { apiFetch } from "./client";

/** applications.Application — a 校系 the student applied to. Unrelated to
 * account-applications.ts (the sign-up request flow). */
export interface Application {
    id: string;
    program_identifier: string;
    school_code: string;
    school_name: string;
    program_code: string;
    program_name: string;
    academic_year: number;
    status: string;
    locked_at?: string;
}

/** "115 學年度 國立臺灣大學 資訊工程學系" */
export const applicationLabel = (application: Application) =>
    `${application.academic_year} 學年度 ${application.school_name} ${application.program_name}`;

/** POST /api/v1/applications accepts 1..50 identifiers per request. */
export const APPLICATION_BATCH_LIMIT = 50;

/** GET /api/v1/applications — needs a verified identity (403
 * verification_required otherwise). */
export function listApplications(options?: { signal?: AbortSignal }) {
    return apiFetch<{ data: Application[] }>("/api/v1/applications", {
        signal: options?.signal
    });
}

/** POST /api/v1/applications — creates confirmed (locked) applications;
 * student identity only. */
export function createApplications(programIdentifiers: string[]) {
    return apiFetch<{ data: Application[] }>("/api/v1/applications", {
        method: "POST",
        body: { program_identifiers: programIdentifiers }
    });
}

export type ResultStatus = "admitted" | "waitlisted" | "rejected" | "unknown";

/** results.Report — 404 result_not_found until the official list is out. */
export interface ApplicationResult {
    application_id: string;
    result_status: ResultStatus;
    official_rank?: number;
    quota?: number;
    front_candidate_count: number;
    front_response_count: number;
    position_after_declines?: number;
    /** Already a percentage (0–100), averaged from willingness values. */
    reference_probability?: number;
    current_willingness?: number;
    candidate_number_last4?: string;
    candidate_number_set: boolean;
}

export interface ApplicationInquiry {
    id: string;
    round: "result_released" | "acceptance_deadline";
    response_deadline?: string;
    created_at: string;
    responded: boolean;
    current_willingness?: number;
}

/** results.validWillingness */
export const WILLINGNESS_VALUES = [0, 20, 40, 60, 80, 100] as const;

/** GET /api/v1/applications/{applicationID}/result */
export function getApplicationResult(applicationID: string, options?: { signal?: AbortSignal }) {
    return apiFetch<{ data: ApplicationResult }>(
        `/api/v1/applications/${encodeURIComponent(applicationID)}/result`,
        { signal: options?.signal }
    );
}

/** GET /api/v1/applications/{applicationID}/inquiries */
export function listApplicationInquiries(
    applicationID: string,
    options?: { signal?: AbortSignal }
) {
    return apiFetch<{ data: ApplicationInquiry[] }>(
        `/api/v1/applications/${encodeURIComponent(applicationID)}/inquiries`,
        { signal: options?.signal }
    );
}

/** PUT /api/v1/applications/{applicationID}/candidate-number — 4–64 chars
 * of [A-Za-z0-9-_]; responds 204. */
export function setCandidateNumber(applicationID: string, candidateNumber: string) {
    return apiFetch<void>(
        `/api/v1/applications/${encodeURIComponent(applicationID)}/candidate-number`,
        { method: "PUT", body: { candidate_number: candidateNumber } }
    );
}

/** PUT /api/v1/applications/{applicationID}/willingness */
export function setWillingness(
    applicationID: string,
    input: { value: (typeof WILLINGNESS_VALUES)[number]; inquiry_id?: string }
) {
    return apiFetch<{ data: unknown }>(
        `/api/v1/applications/${encodeURIComponent(applicationID)}/willingness`,
        { method: "PUT", body: input }
    );
}
