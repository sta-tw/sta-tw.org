import { apiUpload } from "./client";

export interface AccountApplication {
    id: string;
    requested_username: string;
    source: string;
    note: string;
    status: string;
    created_at: string;
}

export interface SubmitAccountApplicationInput {
    username: string;
    email: string;
    note: string;
    files: File[];
}

/**
 * POST /api/v1/account-applications — the no-school-email path. Public,
 * unauthenticated, IP rate-limited. Creates a pending application reviewed
 * by an admin in Telegram; approval emails a password-set link to `email`.
 */
export function submitAccountApplication(input: SubmitAccountApplicationInput) {
    const formData = new FormData();
    formData.set("username", input.username);
    formData.set("email", input.email);
    formData.set("note", input.note);
    for (const file of input.files) {
        formData.append("files", file);
    }
    return apiUpload<{ data: AccountApplication }>("/api/v1/account-applications", formData);
}
