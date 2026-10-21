import { API_BASE_URL, apiFetch, apiUpload } from "./client";
import { ApiError } from "./types";

export type ContributionCategory =
    | "applicants"
    | "second_stage"
    | "waitlist"
    | "historical_brochure";

export type ContributionStatus = "pending" | "accepted" | "rejected";

export interface PublicContributor {
    nickname: string;
    reviewed_at: string;
}

export interface ContributionAttachment {
    id: string;
    file_name: string;
    mime_type: string;
    size_bytes: number;
}

export interface AdmissionsContribution {
    id: string;
    email: string;
    academic_year: number;
    category: ContributionCategory;
    school_name: string;
    data_text: string;
    reference_url: string;
    nickname: string;
    publish_nickname: boolean;
    status: ContributionStatus;
    attachment_name?: string;
    attachment_size_bytes?: number;
    has_attachment: boolean;
    attachments?: ContributionAttachment[];
    reviewed_by?: string;
    reviewed_at?: string;
    created_at: string;
}

export function submitAdmissionsContribution(formData: FormData) {
    return apiUpload<{
        data: {
            id?: string;
            created_at?: string;
            received?: boolean;
            email_queued?: boolean;
            forum_posted?: boolean;
        };
    }>("/api/v1/admissions/contributions", formData);
}

export function listPublicContributors(signal?: AbortSignal) {
    return apiFetch<{ data: PublicContributor[] }>("/api/v1/admissions/contributors", {
        signal
    });
}

export function listAdminAdmissionsContributions(status?: ContributionStatus, mfaCode?: string) {
    return apiFetch<{
        data: AdmissionsContribution[];
        meta: { limit: number; offset: number; count: number };
    }>("/api/v1/admin/admissions/contributions", {
        query: { status, limit: 100 },
        headers: mfaHeaders(mfaCode)
    });
}

export function reviewAdminAdmissionsContribution(
    contributionID: string,
    status: Extract<ContributionStatus, "accepted" | "rejected">,
    mfaCode?: string
) {
    return apiFetch<{ data: AdmissionsContribution }>(
        `/api/v1/admin/admissions/contributions/${encodeURIComponent(contributionID)}/review`,
        {
            method: "POST",
            body: { status },
            headers: mfaHeaders(mfaCode)
        }
    );
}

export async function getAdminAdmissionsContributionContent(
    contributionID: string,
    attachmentID: string,
    mfaCode?: string
): Promise<Blob> {
    let response: Response;
    try {
        response = await fetch(
            `${API_BASE_URL}/api/v1/admin/admissions/contributions/${encodeURIComponent(contributionID)}/attachments/${encodeURIComponent(attachmentID)}/content`,
            {
                credentials: "include",
                headers: { Accept: "application/octet-stream", ...(mfaHeaders(mfaCode) ?? {}) }
            }
        );
    } catch {
        throw new ApiError(0, "network_error", "無法連線到後端服務，請確認服務是否啟動。");
    }
    if (!response.ok) {
        let errorBody: { error?: { code?: string; message?: string } } | undefined;
        try {
            errorBody = (await response.json()) as typeof errorBody;
        } catch {
            // Keep a readable fallback if the backend did not return JSON.
        }
        throw new ApiError(
            response.status,
            errorBody?.error?.code ?? "attachment_download_failed",
            errorBody?.error?.message ?? response.statusText
        );
    }
    return response.blob();
}

function mfaHeaders(code?: string): Record<string, string> | undefined {
    return code ? { "X-MFA-Code": code } : undefined;
}
