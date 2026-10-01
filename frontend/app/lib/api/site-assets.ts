import { API_BASE_URL, apiFetch, apiUpload } from "./client";

export interface SiteAssetMetadata {
    exists: boolean;
    content_type?: string;
    size_bytes?: number;
    updated_at?: string;
}

export const articleOverviewFallbackPath = "/articlemain/article-overview.png";

const articleOverviewImagePath = "/api/v1/site-assets/article-overview";
const articleOverviewImageMetaPath = articleOverviewImagePath + "/meta";
const adminArticleOverviewImagePath = "/api/v1/admin/site-assets/article-overview";

function adminHeaders(mfaCode?: string): Record<string, string> | undefined {
    return mfaCode ? { "X-MFA-Code": mfaCode } : undefined;
}

export function getArticleOverviewImageMeta() {
    return apiFetch<{ data: SiteAssetMetadata }>(articleOverviewImageMetaPath);
}

export function getArticleOverviewImageUrl(version?: string) {
    const query = version ? "?v=" + encodeURIComponent(version) : "";
    return API_BASE_URL + articleOverviewImagePath + query;
}

export function getAdminArticleOverviewImageMeta(mfaCode?: string) {
    return apiFetch<{ data: SiteAssetMetadata }>(adminArticleOverviewImagePath, {
        headers: adminHeaders(mfaCode)
    });
}

export function uploadAdminArticleOverviewImage(formData: FormData, mfaCode?: string) {
    return apiUpload<{ data: SiteAssetMetadata }>(adminArticleOverviewImagePath, formData, {
        headers: adminHeaders(mfaCode)
    });
}

export function resetAdminArticleOverviewImage(mfaCode?: string) {
    return apiFetch<void>(adminArticleOverviewImagePath, {
        method: "DELETE",
        headers: adminHeaders(mfaCode)
    });
}
