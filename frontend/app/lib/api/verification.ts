import { apiFetch, apiUpload } from "./client";
import { mockResponse, mockStore } from "./mock";

export type VerificationStatus = "pending" | "approved" | "rejected" | "cancelled" | "expired";

/** verification.Request from GET /api/v1/verification/requests. */
export interface VerificationRequest {
    id: string;
    academic_year: number;
    school_code: string;
    program_code?: string;
    method: "school_email" | "document";
    status: VerificationStatus;
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
    /** Three-digit school code. */
    school_code: string;
    /** Optional three-digit program code; must exist for that year/school. */
    program_code?: string;
}

/** GET /api/v1/verification/requests. Wrapped below. */
function realGetVerificationRequests(options?: { signal?: AbortSignal }) {
    return apiFetch<{ data: VerificationRequest[] }>("/api/v1/verification/requests", {
        signal: options?.signal
    });
}

/** POST /api/v1/verification/requests/document. Wrapped below. */
function realCreateDocumentRequest(input: DocumentRequestInput) {
    return apiFetch<{ data: VerificationRequest }>("/api/v1/verification/requests/document", {
        method: "POST",
        body: input
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

// ---------------------------------------------------------------------------
// Not backed yet — see lib/api/mock.ts.

export type VerificationIdentityType = "student" | "senior";
export type VerificationDocumentType = "student_id" | "admission_letter" | "other";

/** Fields the document request will carry once the backend stores them. */
export interface VerificationRequestDetails {
    identity_type: VerificationIdentityType;
    document_type: VerificationDocumentType;
    academic_fields: string[];
}

/** A request as the mocked GET will return it: the real row plus the extra
 * fields, and "revoked" for an approved identity the user removed. */
export type VerificationRequestWithDetails = Omit<VerificationRequest, "status"> &
    Partial<VerificationRequestDetails> & { status: VerificationStatus | "revoked" };

type StoredExtras = Partial<VerificationRequestDetails> & { status?: "cancelled" | "revoked" };
const EXTRAS_KEY = "verification-extras";

function readExtras() {
    return mockStore.read<Record<string, StoredExtras>>(EXTRAS_KEY, {});
}

function patchExtras(id: string, patch: StoredExtras) {
    const all = readExtras();
    mockStore.write(EXTRAS_KEY, { ...all, [id]: { ...all[id], ...patch } });
}

/** GET /api/v1/verification/requests, MOCK: merged with the locally
 * stored identity/document/academic-field values and status overrides,
 * newest first. */
export async function getVerificationRequests(options?: { signal?: AbortSignal }) {
    const { data } = await realGetVerificationRequests(options);
    const extras = readExtras();
    const merged: VerificationRequestWithDetails[] = data
        .map((request) => ({ ...request, ...extras[request.id] }))
        .sort((a, b) => b.created_at.localeCompare(a.created_at));
    return { data: merged };
}

/** POST /api/v1/verification/requests/document with MOCK: identity_type,
 * document_type and academic_fields in the body — the real request is
 * created and the extra fields are recorded locally. */
export async function createDocumentRequest(
    input: DocumentRequestInput & VerificationRequestDetails
) {
    const { identity_type, document_type, academic_fields, ...real } = input;
    const { data } = await realCreateDocumentRequest(real);
    patchExtras(data.id, { identity_type, document_type, academic_fields });
    const merged: VerificationRequestWithDetails = {
        ...data,
        identity_type,
        document_type,
        academic_fields
    };
    return { data: merged };
}

/** MOCK: DELETE /api/v1/verification/requests/{id} — withdraws a pending
 * request (204; the server will answer 409 for any other status). */
export function withdrawVerificationRequest(id: string) {
    return mockResponse((): void => patchExtras(id, { status: "cancelled" }));
}

/** MOCK: POST /api/v1/verification/requests/{id}/revoke — removes an
 * approved identity (204; 409 for any other status). */
export function revokeVerification(id: string) {
    return mockResponse((): void => patchExtras(id, { status: "revoked" }));
}

/** Full document submission: withdraws the pending request it replaces (if
 * any), creates the request, then uploads the file. If the upload fails the
 * new, empty request is withdrawn so it doesn't sit in the review queue.
 * MOCK: both withdraw steps use the mocked DELETE above, so until
 * it is real a replaced or failed request stays pending on the server. */
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
