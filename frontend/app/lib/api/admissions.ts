import { API_BASE_URL, apiFetch, apiUpload } from "./client";

export interface AdmissionExamItem {
    name: string;
    stage: string;
    sort_order: number;
    weight_percent?: number;
    multiplier?: number;
    description: string;
    source_page: string;
}

/** One row of a program's admissions schedule (網路登錄報名, 初試結果公告,
 * 複試繳費期限, 榜單公告, 備取遞補截止, ...), in publication order. A blank
 * end_date means a one-day/point-in-time milestone (only start_date/
 * start_time matter); a set end_date means it spans a range. start_time/
 * end_time are "HH:MM" or "-" when the brochure gives no time of day. */
export interface AdmissionTimelineEvent {
    name: string;
    start_date: string;
    start_time: string;
    end_date: string;
    end_time: string;
    sort_order: number;
    notes: string;
}

export interface AdmissionProgram {
    academic_year: number;
    program_identifier: string;
    school_code: string;
    school_name: string;
    program_code: string;
    admission_program_name: string;
    admission_quota: number;
    willingness_values: number[];
    exam_items: AdmissionExamItem[];
    timeline_events: AdmissionTimelineEvent[];
    brochure_is_tentative: boolean;
    consultation_phone: string;
    consultation_email: string;
    consultation_contact: string;
    brochure_url: string;
    special_talent_target: string;
    different_education_backgrounds: string;
    different_education_other: string;
    notes: string;
    source_locator: string;
    registration_fee: string;
    exam_location: string;
    /** "-" means not required; any other value is the deadline and implies required. */
    recommendation_letter_deadline: string;
    /** "-" means not required; any other value is the deadline and implies required. */
    portfolio_deadline: string;
    checkin_waitlist_process: string;
    fee_reduction_eligibility: string;
    /** These mirror the community-maintained "大表" (master spreadsheet)
     * columns that are now the primary data-entry source. Free text like
     * everything else here, since the sheet itself mixes values with
     * explanatory notes (e.g. "Y/不同教育資歷", "尚未公告"). */
    admission_group: string;
    cross_group: string;
    admission_category: string;
    priority_admission: string;
    portfolio_required: string;
    recommendation_letter_type: string;
    max_applicable_programs: string;
    applicant_count: string;
    interview_count: string;
    admitted_count: string;
    waitlisted_count: string;
    admission_rate: string;
    first_stage_pass_rate: string;
    competition_ratio: string;
    /** The school's/department's own homepage — distinct from brochure_url,
     * which is the admissions/簡章 info page specifically. */
    school_official_url: string;
    department_official_url: string;
}

export interface AdmissionSchool {
    school_code: string;
    school_name: string;
}

export interface BrochureDocument {
    academic_year: number;
    school_code: string;
    original_file_name: string;
    mime_type: string;
    file_size_bytes: number;
    sha256: string;
    source_url: string;
    review_status: string;
    published_at?: string;
    created_at: string;
    updated_at: string;
}

export interface AdmissionListResponse<T> {
    data: T[];
    meta?: {
        limit?: number;
        offset?: number;
        count?: number;
    };
}

export interface BrochureDownloadResponse {
    data: BrochureDocument;
    url: string;
    expires_in: number;
}

export type ProgramReviewStatus =
    "draft" | "pending" | "approved" | "published" | "rejected" | "archived";

export interface AdminAdmissionProgram extends AdmissionProgram {
    review_status: ProgramReviewStatus;
    created_at: string;
    updated_at: string;
}

/** Fields accepted by the admin sync/update endpoints. */
export interface AdmissionProgramInput {
    academic_year: number;
    school_code: string;
    program_code: string;
    admission_program_name: string;
    admission_quota: number;
    exam_items: AdmissionExamItem[];
    timeline_events: AdmissionTimelineEvent[];
    brochure_is_tentative: boolean;
    consultation_phone: string;
    consultation_email: string;
    consultation_contact: string;
    brochure_url: string;
    special_talent_target: string;
    different_education_backgrounds: string;
    different_education_other: string;
    notes: string;
    source_page?: number;
    registration_fee: string;
    exam_location: string;
    recommendation_letter_deadline: string;
    portfolio_deadline: string;
    checkin_waitlist_process: string;
    fee_reduction_eligibility: string;
    admission_group: string;
    cross_group: string;
    admission_category: string;
    priority_admission: string;
    portfolio_required: string;
    recommendation_letter_type: string;
    max_applicable_programs: string;
    applicant_count: string;
    interview_count: string;
    admitted_count: string;
    waitlisted_count: string;
    admission_rate: string;
    first_stage_pass_rate: string;
    competition_ratio: string;
    school_official_url: string;
    department_official_url: string;
}

export interface ProgramAuditEvent {
    id: number;
    action: string;
    entity_key: string;
    before_data?: Record<string, unknown>;
    after_data?: Record<string, unknown>;
    reason: string;
    created_at: string;
}

export interface BrochureEvent {
    id: number;
    academic_year: number;
    school_code: string;
    action: string;
    from_status?: string;
    to_status?: string;
    original_file_name: string;
    sha256: string;
    reason: string;
    created_at: string;
}

export interface BrochureUploadCandidate {
    program_code: string;
    data: Record<string, unknown>;
    source_page?: number;
    confidence?: number;
}

export type BrochureUploadChannel = "admin_upload" | "external_api";

export interface BrochureUpload {
    id: string;
    job_id: string;
    original_file_name: string;
    mime_type: string;
    file_size_bytes: number;
    sha256: string;
    source_url: string;
    intake_channel: BrochureUploadChannel;
    detected_academic_year?: number;
    detected_school_code?: string;
    detected_school_name?: string;
    status: "queued" | "processing" | "pending_review" | "approved" | "rejected" | "failed";
    error_code?: string;
    error_message?: string;
    created_at: string;
    updated_at: string;
    reviewed_at?: string;
    candidates?: BrochureUploadCandidate[];
}

export interface BrochureUploadReceipt {
    upload_id: string;
    job_id: string;
    original_file_name: string;
    mime_type: string;
    file_size_bytes: number;
    sha256: string;
    status: string;
}

export interface AdminBrochureUploadResponse {
    data: BrochureUploadReceipt;
    job_id: string;
    extraction_status?: string;
}

export function listAdmissionSchools(options?: { academicYear?: number; signal?: AbortSignal }) {
    return apiFetch<AdmissionListResponse<AdmissionSchool>>("/api/v1/admissions/schools", {
        query: { academic_year: options?.academicYear },
        signal: options?.signal
    });
}

export function listAdmissionPrograms(
    options: {
        academicYear?: number;
        schoolCode?: string;
        programCode?: string;
        q?: string;
        limit?: number;
        offset?: number;
        signal?: AbortSignal;
    } = {}
) {
    return apiFetch<AdmissionListResponse<AdmissionProgram>>("/api/v1/admissions/programs", {
        query: {
            academic_year: options.academicYear,
            school_code: options.schoolCode,
            program_code: options.programCode,
            q: options.q,
            limit: options.limit,
            offset: options.offset
        },
        signal: options.signal
    });
}

/**
 * All published years of the same school+program (matched by
 * school_code+program_code, which stays stable across a program's yearly
 * re-imports) — the data source for the "歷年招生資料" tab. Sorted by
 * academic_year, most recent first, same as the backend's default order.
 */
export function getAdmissionProgramHistory(
    schoolCode: string,
    programCode: string,
    options?: { signal?: AbortSignal }
) {
    return apiFetch<AdmissionListResponse<AdmissionProgram>>("/api/v1/admissions/programs", {
        query: { school_code: schoolCode, program_code: programCode, limit: 100 },
        signal: options?.signal
    });
}

/**
 * Loads every published program matching the query. The API uses offset
 * pagination, so the brochure index must walk all pages instead of silently
 * showing only the first 100 records.
 */
export async function listAllAdmissionPrograms(
    options: {
        academicYear?: number;
        schoolCode?: string;
        q?: string;
        signal?: AbortSignal;
    } = {}
) {
    const pageSize = 100;
    const programs: AdmissionProgram[] = [];
    let offset = 0;

    while (true) {
        const response = await listAdmissionPrograms({
            ...options,
            limit: pageSize,
            offset
        });
        programs.push(...response.data);

        if (response.data.length < pageSize || offset >= 10_000) break;
        offset += response.data.length;
    }

    return { data: programs };
}

export function getAdmissionProgram(identifier: string, options?: { signal?: AbortSignal }) {
    return apiFetch<{ data: AdmissionProgram }>(
        `/api/v1/admissions/programs/${encodeURIComponent(identifier)}`,
        { signal: options?.signal }
    );
}

/**
 * Every published academic_year's brochure PDF for a school — a school's
 * brochure covers all its departments, so this is keyed by school, same as
 * getPublishedBrochureDownload. Used for "歷史簡章" (past years' brochures);
 * each entry needs its own getPublishedBrochureDownload call to get a fresh
 * presigned URL when the user actually clicks it (the URL expires quickly).
 */
export function listPublishedBrochures(schoolCode: string, options?: { signal?: AbortSignal }) {
    return apiFetch<AdmissionListResponse<BrochureDocument>>(
        `/api/v1/admissions/brochures/${encodeURIComponent(schoolCode)}`,
        { signal: options?.signal }
    );
}

/** Returns a short-lived signed URL for the currently published school brochure. */
export function getPublishedBrochureDownload(
    academicYear: number,
    schoolCode: string,
    options?: { signal?: AbortSignal }
) {
    return apiFetch<BrochureDownloadResponse>(
        `/api/v1/admissions/brochures/${academicYear}/${encodeURIComponent(schoolCode)}/download`,
        { signal: options?.signal }
    );
}

// --- admin admissions ---------------------------------------------------

export interface AdminAdmissionProgramFilter {
    academicYear?: number;
    schoolCode?: string;
    programCode?: string;
    reviewStatus?: ProgramReviewStatus;
    q?: string;
    limit?: number;
    offset?: number;
    signal?: AbortSignal;
}

export function listAdminAdmissionPrograms(
    filter: AdminAdmissionProgramFilter = {},
    mfaCode?: string
) {
    return apiFetch<AdmissionListResponse<AdminAdmissionProgram>>(
        "/api/v1/admin/admissions/programs",
        {
            query: {
                academic_year: filter.academicYear,
                school_code: filter.schoolCode,
                program_code: filter.programCode,
                review_status: filter.reviewStatus,
                q: filter.q,
                limit: filter.limit,
                offset: filter.offset
            },
            headers: adminHeaders(mfaCode),
            signal: filter.signal
        }
    );
}

export function getAdminAdmissionProgram(identifier: string, mfaCode?: string) {
    return apiFetch<{ data: AdminAdmissionProgram }>(
        `/api/v1/admin/admissions/programs/${encodeURIComponent(identifier)}`,
        { headers: adminHeaders(mfaCode) }
    );
}

export function listAdminAdmissionProgramHistory(identifier: string, mfaCode?: string) {
    return apiFetch<{ data: ProgramAuditEvent[] }>(
        `/api/v1/admin/admissions/programs/${encodeURIComponent(identifier)}/history`,
        { headers: adminHeaders(mfaCode) }
    );
}

export function syncAdminAdmissionPrograms(
    reason: string,
    items: AdmissionProgramInput[],
    mfaCode?: string
) {
    return apiFetch<{ data: AdminAdmissionProgram[]; meta?: { count?: number } }>(
        "/api/v1/admin/admissions/programs/sync",
        {
            method: "POST",
            body: { reason, items },
            headers: adminHeaders(mfaCode)
        }
    );
}

export function updateAdminAdmissionProgram(
    identifier: string,
    reason: string,
    item: AdmissionProgramInput,
    mfaCode?: string
) {
    return apiFetch<{ data: AdminAdmissionProgram }>(
        `/api/v1/admin/admissions/programs/${encodeURIComponent(identifier)}`,
        {
            method: "PUT",
            body: { reason, item },
            headers: adminHeaders(mfaCode)
        }
    );
}

export function reviewAdminAdmissionProgram(
    identifier: string,
    approved: boolean,
    reason: string,
    mfaCode?: string
) {
    return apiFetch<{ data: AdminAdmissionProgram }>(
        `/api/v1/admin/admissions/programs/${encodeURIComponent(identifier)}/review`,
        {
            method: "POST",
            body: { approved, reason },
            headers: adminHeaders(mfaCode)
        }
    );
}

export function listAdminBrochures(academicYear?: number, mfaCode?: string) {
    return apiFetch<AdmissionListResponse<BrochureDocument>>("/api/v1/admin/admissions/brochures", {
        query: { academic_year: academicYear },
        headers: adminHeaders(mfaCode)
    });
}

export function listAdminBrochureEvents(
    academicYear: number,
    schoolCode: string,
    mfaCode?: string
) {
    return apiFetch<{ data: BrochureEvent[] }>(
        `/api/v1/admin/admissions/brochures/${academicYear}/${encodeURIComponent(schoolCode)}/events`,
        { headers: adminHeaders(mfaCode) }
    );
}

export function getAdminBrochureDownload(
    academicYear: number,
    schoolCode: string,
    mfaCode?: string
) {
    return apiFetch<BrochureDownloadResponse>(
        `/api/v1/admin/admissions/brochures/${academicYear}/${encodeURIComponent(schoolCode)}/download`,
        { headers: adminHeaders(mfaCode) }
    );
}

export function uploadAdminBrochure(formData: FormData, mfaCode?: string) {
    return apiUpload<AdminBrochureUploadResponse>("/api/v1/admin/admissions/brochures", formData, {
        headers: adminHeaders(mfaCode)
    });
}

export function listAdminBrochureUploads(status?: BrochureUpload["status"], mfaCode?: string) {
    return apiFetch<AdmissionListResponse<BrochureUpload>>(
        "/api/v1/admin/admissions/brochure-uploads",
        {
            query: { status },
            headers: adminHeaders(mfaCode)
        }
    );
}

export function getAdminBrochureUpload(uploadID: string, mfaCode?: string) {
    return apiFetch<{ data: BrochureUpload }>(
        `/api/v1/admin/admissions/brochure-uploads/${encodeURIComponent(uploadID)}`,
        { headers: adminHeaders(mfaCode) }
    );
}

export function getAdminBrochureUploadDownload(uploadID: string, mfaCode?: string) {
    return apiFetch<{ url: string; expires_in: number }>(
        `/api/v1/admin/admissions/brochure-uploads/${encodeURIComponent(uploadID)}/download`,
        { headers: adminHeaders(mfaCode) }
    );
}

export function getAdminBrochureUploadContentURL(uploadID: string) {
    return `${API_BASE_URL}/api/v1/admin/admissions/brochure-uploads/${encodeURIComponent(uploadID)}/content`;
}

export function getAdminBrochureContentURL(academicYear: number, schoolCode: string) {
    return `${API_BASE_URL}/api/v1/admin/admissions/brochures/${academicYear}/${encodeURIComponent(schoolCode)}/content`;
}

export function confirmAdminBrochureUpload(
    uploadID: string,
    input: {
        academic_year: number;
        school_code: string;
        programs: AdmissionProgramInput[];
        reason?: string;
    },
    mfaCode?: string
) {
    return apiFetch<{
        data: {
            upload: BrochureUpload;
            brochure: BrochureDocument;
            programs: AdminAdmissionProgram[];
        };
    }>(`/api/v1/admin/admissions/brochure-uploads/${encodeURIComponent(uploadID)}/confirm`, {
        method: "POST",
        body: input,
        headers: adminHeaders(mfaCode)
    });
}

export function rejectAdminBrochureUpload(uploadID: string, reason: string, mfaCode?: string) {
    return apiFetch<{ data: BrochureUpload }>(
        `/api/v1/admin/admissions/brochure-uploads/${encodeURIComponent(uploadID)}/reject`,
        {
            method: "POST",
            body: { reason },
            headers: adminHeaders(mfaCode)
        }
    );
}

export function reviewAdminBrochure(
    academicYear: number,
    schoolCode: string,
    approved: boolean,
    reason: string,
    mfaCode?: string
) {
    return apiFetch<{ data: BrochureDocument }>(
        `/api/v1/admin/admissions/brochures/${academicYear}/${encodeURIComponent(schoolCode)}/review`,
        {
            method: "POST",
            body: { approved, reason },
            headers: adminHeaders(mfaCode)
        }
    );
}

export function setAdminBrochureVisibility(
    academicYear: number,
    schoolCode: string,
    published: boolean,
    reason: string,
    mfaCode?: string
) {
    return apiFetch<{ data: BrochureDocument }>(
        `/api/v1/admin/admissions/brochures/${academicYear}/${encodeURIComponent(schoolCode)}/visibility`,
        {
            method: "POST",
            body: { published, reason },
            headers: adminHeaders(mfaCode)
        }
    );
}

function adminHeaders(mfaCode?: string): Record<string, string> | undefined {
    return mfaCode ? { "X-MFA-Code": mfaCode } : undefined;
}
