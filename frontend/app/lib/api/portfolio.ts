import { apiFetch, apiUpload } from "./client";
import { listApplications, type Application } from "./applications";
import { ApiError } from "./types";

/** portfolio.Project — one per confirmed application. */
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

/** Each upload is a new version of the project's single file. */
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

export const PORTFOLIO_FILE_MAX_BYTES = 50 * 1024 * 1024;
export const PORTFOLIO_FILE_ACCEPT =
    ".pdf,.doc,.docx,.ppt,.pptx,.xls,.xlsx,.csv,.tsv,.txt,.json,.jpg,.jpeg,.png,.gif,.webp";
export const PORTFOLIO_TITLE_MAX_LENGTH = 200;

/** The card model adapts the backend's application/project/file structure. */
export type PortfolioVisibility = "private" | "public";
export type PortfolioReviewStatus = "draft" | "pending_review" | "approved" | "rejected";

export interface PortfolioItem {
    id: string;
    name: string;
    file_name: string;
    file_size_bytes: number;
    mime_type: string;
    version_number: number;
    status: PortfolioFileStatus;
    updated_at: string;
}

export interface Portfolio {
    id: string;
    application_id: string;
    title: string;
    academic_year: number;
    school_name: string;
    program_name: string;
    visibility: PortfolioVisibility;
    review_status: PortfolioReviewStatus;
    rejection_reason?: string;
    items: PortfolioItem[];
    author_username: string;
    author_name: string;
    like_count: number;
    liked: boolean;
    saved: boolean;
    created_at: string;
    updated_at: string;
}

export const portfolioVisibilityLabels: Record<PortfolioVisibility, string> = {
    private: "尚未公開",
    public: "公開"
};

export const portfolioLabel = (portfolio: Pick<Portfolio, "school_name" | "program_name">) =>
    `${portfolio.school_name} ${portfolio.program_name}`.trim();

export const portfolioUpdatedAt = (portfolio: Portfolio) =>
    portfolio.items.reduce(
        (latest, item) => (item.updated_at > latest ? item.updated_at : latest),
        portfolio.updated_at
    );

/** GET /api/v1/portfolio/projects — requires a verified identity. */
export function listPortfolioProjects(options?: { signal?: AbortSignal }) {
    return apiFetch<{ data: PortfolioProject[] }>("/api/v1/portfolio/projects", {
        signal: options?.signal
    });
}

/** POST /api/v1/portfolio/projects — ties one project to an owned application. */
export function createPortfolioProject(input: { application_id: string; title: string }) {
    return apiFetch<{ data: PortfolioProject }>("/api/v1/portfolio/projects", {
        method: "POST",
        body: input
    });
}

/** GET /api/v1/portfolio/projects/{projectID}/files. */
export function listPortfolioFiles(projectID: string, options?: { signal?: AbortSignal }) {
    return apiFetch<{ data: PortfolioFile[] }>(
        `/api/v1/portfolio/projects/${encodeURIComponent(projectID)}/files`,
        { signal: options?.signal }
    );
}

/** POST /api/v1/portfolio/projects/{projectID}/files — creates a hidden version. */
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

export const submitPortfolioFile = (fileID: string) => changeFileStatus(fileID, "submit");
export const unpublishPortfolioFile = (fileID: string) => changeFileStatus(fileID, "unpublish");
export const hidePortfolioFile = (fileID: string) => changeFileStatus(fileID, "hide");

/** GET /api/v1/portfolio/files/{fileID}/download — signed URL. */
export function getPortfolioFileDownload(fileID: string) {
    return apiFetch<{ url: string; expires_in: number }>(
        `/api/v1/portfolio/files/${encodeURIComponent(fileID)}/download`
    );
}

/** Compatibility name used by the portfolio card. */
export const getPortfolioItemDownload = getPortfolioFileDownload;

function statusToReview(status?: PortfolioFileStatus): PortfolioReviewStatus {
    if (status === "pending_review") return "pending_review";
    if (status === "published") return "approved";
    if (status === "rejected") return "rejected";
    return "draft";
}

function toPortfolioItem(file: PortfolioFile): PortfolioItem {
    return {
        id: file.id,
        name: file.original_file_name,
        file_name: file.original_file_name,
        file_size_bytes: file.file_size_bytes,
        mime_type: file.mime_type,
        version_number: file.version_number,
        status: file.status,
        updated_at: file.updated_at
    };
}

function projectToPortfolio(
    project: PortfolioProject,
    application: Application,
    files: PortfolioFile[],
    authorUsername = ""
): Portfolio {
    const sorted = [...files].sort((a, b) => b.version_number - a.version_number);
    const latest = sorted[0];
    const rejection = sorted.find((file) => file.status === "rejected")?.rejection_reason;
    return {
        id: project.id,
        application_id: project.application_id,
        title: project.title,
        academic_year: application.academic_year,
        school_name: application.school_name,
        program_name: application.program_name,
        visibility: files.some((file) => file.status === "published") ? "public" : "private",
        review_status: statusToReview(latest?.status),
        rejection_reason: rejection,
        items: sorted.map(toPortfolioItem),
        author_username: authorUsername,
        author_name: authorUsername,
        like_count: 0,
        liked: false,
        saved: false,
        created_at: project.created_at,
        updated_at: project.updated_at
    };
}

/** Own projects and their version history, joined to the account's applications. */
export async function listMyPortfolios(options?: { signal?: AbortSignal }) {
    const [{ data: projects }, { data: applications }] = await Promise.all([
        listPortfolioProjects(options),
        listApplications(options)
    ]);
    const applicationsByID = new Map(
        applications.map((application) => [application.id, application])
    );
    const data = await Promise.all(
        projects.map(async (project) => {
            const application = applicationsByID.get(project.application_id);
            if (!application) return null;
            const { data: files } = await listPortfolioFiles(project.id, options);
            return projectToPortfolio(project, application, files);
        })
    );
    return { data: data.filter((portfolio): portfolio is Portfolio => portfolio !== null) };
}

export async function getMyPortfolio(portfolioID: string, options?: { signal?: AbortSignal }) {
    const { data } = await listMyPortfolios(options);
    const portfolio = data.find((item) => item.id === portfolioID);
    if (!portfolio) throw new ApiError(404, "not_found", "找不到這份備審資料。");
    return { data: portfolio };
}

/** Submit the newest hidden, unpublished or rejected file version for review. */
export async function submitPortfolio(portfolioID: string) {
    const { data: portfolio } = await getMyPortfolio(portfolioID);
    const newest = portfolio.items[0];
    const file =
        newest && ["hidden", "unpublished", "rejected"].includes(newest.status)
            ? newest
            : undefined;
    if (!file) throw new ApiError(409, "invalid_state", "目前沒有可送審的檔案版本。");
    const { data } = await submitPortfolioFile(file.id);
    return {
        data: {
            ...portfolio,
            items: portfolio.items.map((item) =>
                item.id === data.id ? toPortfolioItem(data) : item
            ),
            review_status: "pending_review" as const
        }
    };
}

/** GET /api/v1/users/{username}/portfolio — only published files visible on the public profile. */
export async function listUserPortfolios(username: string, options?: { signal?: AbortSignal }) {
    const result = await apiFetch<{
        data: {
            file_id: string;
            project_title: string;
            school_name: string;
            program_name: string;
            academic_year: number;
            published_at: string;
        }[];
    }>(`/api/v1/users/${encodeURIComponent(username)}/portfolio`, { signal: options?.signal });
    return {
        data: result.data.map(
            (item): Portfolio => ({
                id: item.file_id,
                application_id: "",
                title: item.project_title,
                academic_year: item.academic_year,
                school_name: item.school_name,
                program_name: item.program_name,
                visibility: "public",
                review_status: "approved",
                items: [
                    {
                        id: item.file_id,
                        name: item.project_title,
                        file_name: item.project_title,
                        file_size_bytes: 0,
                        mime_type: "application/octet-stream",
                        version_number: 0,
                        status: "published",
                        updated_at: item.published_at
                    }
                ],
                author_username: username,
                author_name: username,
                like_count: 0,
                liked: false,
                saved: false,
                created_at: item.published_at,
                updated_at: item.published_at
            })
        )
    };
}

export function deletePortfolio(_portfolioID: string): Promise<void> {
    void _portfolioID;
    return Promise.reject(new ApiError(501, "not_supported", "正式後端目前尚未提供備審刪除功能。"));
}
