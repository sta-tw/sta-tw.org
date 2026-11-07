import { apiFetch, apiUpload } from "./client";

/** portfolio.Project — one per application (備審資料 for that 校系). */
export interface PortfolioProject {
    id: string;
    application_id: string;
    title: string;
    created_at: string;
    updated_at: string;
}

export type PortfolioFileStatus =
    | "hidden"
    | "pending_review"
    | "published"
    | "unpublished"
    | "rejected";

/** portfolio.File — every upload is a new version of the project. */
export interface PortfolioFile {
    id: string;
    project_id: string;
    version_number: number;
    original_file_name: string;
    mime_type: string;
    file_size_bytes: number;
    sha256_hex: string;
    status: PortfolioFileStatus;
    rejection_reason?: string;
    created_at: string;
    updated_at: string;
}

/** storage.MaxPortfolioFileBytes in the backend (50 MiB). */
export const PORTFOLIO_FILE_MAX_BYTES = 50 * 1024 * 1024;

/** Extensions storage.validateFileType accepts. */
export const PORTFOLIO_FILE_ACCEPT =
    ".pdf,.doc,.docx,.ppt,.pptx,.xls,.xlsx,.csv,.tsv,.txt,.json,.jpg,.jpeg,.png,.gif,.webp";

/** Project title limit enforced by POST /portfolio/projects. */
export const PORTFOLIO_TITLE_MAX_LENGTH = 200;

/** GET /api/v1/portfolio/projects — needs a verified identity (403
 * verification_required otherwise). */
export function listPortfolioProjects(options?: { signal?: AbortSignal }) {
    return apiFetch<{ data: PortfolioProject[] }>("/api/v1/portfolio/projects", {
        signal: options?.signal
    });
}

/** POST /api/v1/portfolio/projects — the application must be confirmed and
 * owned by the caller (404 otherwise, 409 if it already has a project). */
export function createPortfolioProject(input: { application_id: string; title: string }) {
    return apiFetch<{ data: PortfolioProject }>("/api/v1/portfolio/projects", {
        method: "POST",
        body: input
    });
}

/** GET /api/v1/portfolio/projects/{projectID}/files */
export function listPortfolioFiles(projectID: string, options?: { signal?: AbortSignal }) {
    return apiFetch<{ data: PortfolioFile[] }>(
        `/api/v1/portfolio/projects/${encodeURIComponent(projectID)}/files`,
        { signal: options?.signal }
    );
}

/** POST /api/v1/portfolio/projects/{projectID}/files — multipart `file`;
 * creates a new hidden version. */
export function uploadPortfolioFile(projectID: string, file: File) {
    const formData = new FormData();
    formData.set("file", file);
    return apiUpload<{ data: PortfolioFile }>(
        `/api/v1/portfolio/projects/${encodeURIComponent(projectID)}/files`,
        formData
    );
}

function changeFileStatus(fileID: string, action: "submit" | "unpublish" | "hide") {
    return apiFetch<{ data: PortfolioFile }>(
        `/api/v1/portfolio/files/${encodeURIComponent(fileID)}/${action}`,
        { method: "POST" }
    );
}

/** POST /api/v1/portfolio/files/{fileID}/submit — hidden / unpublished /
 * rejected → pending_review. */
export const submitPortfolioFile = (fileID: string) => changeFileStatus(fileID, "submit");

/** POST /api/v1/portfolio/files/{fileID}/unpublish — published → unpublished. */
export const unpublishPortfolioFile = (fileID: string) => changeFileStatus(fileID, "unpublish");

/** POST /api/v1/portfolio/files/{fileID}/hide — published / unpublished /
 * rejected → hidden. */
export const hidePortfolioFile = (fileID: string) => changeFileStatus(fileID, "hide");

/** GET /api/v1/portfolio/files/{fileID}/download — a short-lived presigned
 * URL (note: not wrapped in `data`). */
export function getPortfolioFileDownload(fileID: string) {
    return apiFetch<{ url: string; expires_in: number }>(
        `/api/v1/portfolio/files/${encodeURIComponent(fileID)}/download`
    );
}
