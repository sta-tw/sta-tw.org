import { apiFetch, apiUpload } from "./client";

export type VerificationStatus =
    | "pending"
    | "approved"
    | "rejected"
    | "cancelled"
    | "expired"
    | "revoked";

/** verification.Request from GET /api/v1/verification/requests. */
export interface VerificationRequest {
    id: string;
    applicant_username?: string;
    academic_year: number;
    school_code?: string;
    school_name?: string;
    school_directory_id?: string;
    program_code?: string;
    program_name?: string;
    method: "school_email" | "document";
    status: VerificationStatus;
    identity_type: VerificationIdentityType;
    document_type: VerificationDocumentType;
    academic_fields: string[];
    document_count: number;
    created_at: string;
    reviewed_at?: string;
}

/** verification.Document from POST …/requests/{id}/documents. */
export interface VerificationDocument {
    id: string;
    request_id: string;
    original_file_name: string;
    mime_type: string;
    file_size_bytes: number;
    sha256: string;
    status: "pending" | "approved" | "rejected";
    rejection_reason?: string;
    reviewed_at?: string;
    created_at: string;
}

interface DocumentRequestInput {
    academic_year: number;
    /** Three-digit university code for non-current-student requests. */
    school_code?: string;
    /** Account-school directory ID for a current student's high school. */
    school_directory_id?: string;
    /** Optional three-digit program code; must exist for that year/school. */
    program_code?: string;
}

/** GET /api/v1/verification/requests. Wrapped below. */
function realGetVerificationRequests(options?: { signal?: AbortSignal }) {
    return apiFetch<{ data: VerificationRequest[] }>("/api/v1/verification/requests", {
        signal: options?.signal
    });
}

/** POST /api/v1/verification/requests/{id}/documents — multipart `file`. */
function uploadVerificationDocument(requestId: string, file: File) {
    const form = new FormData();
    form.append("file", file);
    return apiUpload<{ data: VerificationDocument }>(
        `/api/v1/verification/requests/${encodeURIComponent(requestId)}/documents`,
        form
    );
}

export const VERIFICATION_DOCUMENT_ACCEPT = ".pdf,.png,.jpg,.jpeg";
export const VERIFICATION_DOCUMENT_MAX_BYTES = 50 * 1024 * 1024;

export type VerificationIdentityType = "student" | "senior" | "parent" | "educator";
export type VerificationDocumentType =
    | "student_id"
    | "admission_letter"
    | "alumni_proof"
    | "parent_relationship"
    | "educator_credential"
    | "other";

/** Fields the document request will carry once the backend stores them. */
export interface VerificationRequestDetails {
    identity_type: VerificationIdentityType;
    document_type: VerificationDocumentType;
    academic_fields: string[];
}

/** A request whose approved verification has been revoked by its owner. */
export type VerificationRequestWithDetails = VerificationRequest;

/** GET /api/v1/verification/requests — newest first. */
export function getVerificationRequests(options?: { signal?: AbortSignal }) {
    return realGetVerificationRequests(options) as Promise<{
        data: VerificationRequestWithDetails[];
    }>;
}

/** POST /api/v1/verification/requests/document. */
export function createDocumentRequest(input: DocumentRequestInput & VerificationRequestDetails) {
    return apiFetch<{ data: VerificationRequestWithDetails }>(
        "/api/v1/verification/requests/document",
        {
            method: "POST",
            body: input
        }
    );
}

/** DELETE /api/v1/verification/requests/{id} — withdraws a pending request. */
export function withdrawVerificationRequest(id: string) {
    return apiFetch<void>(`/api/v1/verification/requests/${encodeURIComponent(id)}`, {
        method: "DELETE"
    });
}

/** POST /api/v1/verification/requests/{id}/revoke — removes an approved identity. */
export function revokeVerification(id: string) {
    return apiFetch<void>(`/api/v1/verification/requests/${encodeURIComponent(id)}/revoke`, {
        method: "POST"
    });
}

function adminVerificationHeaders(code?: string): Record<string, string> | undefined {
    return code ? { "X-MFA-Code": code } : undefined;
}

/** GET /api/v1/admin/verification/requests/pending — document requests only. */
export function listPendingAdminVerificationRequests(mfaCode?: string) {
    return apiFetch<{ data: VerificationRequest[] }>(
        "/api/v1/admin/verification/requests/pending",
        { headers: adminVerificationHeaders(mfaCode) }
    );
}

/** GET /api/v1/admin/verification/requests/{id}/documents. */
export function listAdminVerificationDocuments(requestId: string, mfaCode?: string) {
    return apiFetch<{ data: VerificationDocument[] }>(
        `/api/v1/admin/verification/requests/${encodeURIComponent(requestId)}/documents`,
        { headers: adminVerificationHeaders(mfaCode) }
    );
}

/** GET .../documents/{id}/download — returns a short lived private URL. */
export function getAdminVerificationDocumentPreview(
    requestId: string,
    documentId: string,
    mfaCode?: string
) {
    return apiFetch<{ data: VerificationDocument; url: string; expires_in: number }>(
        `/api/v1/admin/verification/requests/${encodeURIComponent(requestId)}/documents/${encodeURIComponent(documentId)}/download`,
        { headers: adminVerificationHeaders(mfaCode) }
    );
}

/** POST /api/v1/admin/verification/requests/{id}/review. */
export function reviewAdminVerificationRequest(
    requestId: string,
    input: { approved: boolean; reason: string },
    mfaCode?: string
) {
    return apiFetch<{ data: { request: VerificationRequest } }>(
        `/api/v1/admin/verification/requests/${encodeURIComponent(requestId)}/review`,
        {
            method: "POST",
            body: input,
            headers: adminVerificationHeaders(mfaCode)
        }
    );
}

/** Full document submission: withdraws the pending request it replaces (if
 * any), creates the request, then uploads the file. If the upload fails the
 * new, empty request is withdrawn so it doesn't sit in the review queue.
 * Both withdraw steps use the server DELETE endpoint, so replaced or failed
 * submissions do not remain pending in the review queue. */
export async function submitDocumentVerification(
    input: DocumentRequestInput & VerificationRequestDetails,
    file: File,
    replacing?: string
) {
    if (replacing) await withdrawVerificationRequest(replacing);
    const { data: request } = await createDocumentRequest(input);
    try {
        await uploadVerificationDocument(request.id, file);
    } catch (cause) {
        await withdrawVerificationRequest(request.id).catch(() => undefined);
        throw cause;
    }
    return request;
}
