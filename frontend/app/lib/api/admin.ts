import { apiFetch } from "./client";
import type { Page } from "./forum";
import type { SitePolicyDocuments, SitePolicyKey } from "./site-policies";

/** GET /api/v1/admin/stats — one snapshot of platform counters + outbox health. */
export interface OutboxHealth {
    pending: number;
    failed: number;
    abandoned: number;
}

export interface AdminStats {
    generated_at: string;
    accounts: {
        total: number;
        active: number;
        suspended: number;
        deleted: number;
        students: number;
        seniors: number;
        verified: number;
    };
    applications: {
        total: number;
        draft: number;
        confirmed: number;
        withdrawn: number;
        archived: number;
    };
    experiences: { total: number; published: number; hidden: number; unpublished: number };
    forum: { spaces: number; threads: number; posts: number };
    chat: { lounge_messages: number };
    support_tickets: { total: number; open: number; closed: number };
    verification_requests: { pending: number; approved: number; rejected: number };
    result_batches: { total: number; pending_review: number; published: number };
    audit_log: { total: number };
    outbox: {
        email: OutboxHealth;
        chat_sync: OutboxHealth;
        support_discord: OutboxHealth;
        willingness_notifications: OutboxHealth;
    };
}

export function getStats(mfaCode?: string) {
    return apiFetch<AdminStats>("/api/v1/admin/stats", { headers: mfaHeader(mfaCode) });
}

// --- users --------------------------------------------------------------

export type AccountStatus = "active" | "pending_verification" | "suspended" | "deleted";
export type IdentityStatus = "temporary" | "student" | "senior";

export interface AdminUser {
    id: string;
    username: string;
    identity_status: IdentityStatus;
    account_status: AccountStatus;
    email_verified: boolean;
    is_admin: boolean;
    is_admissions_moderator: boolean;
    last_login_at?: string;
    suspended_at?: string;
    suspension_reason?: string;
    created_at: string;
}

export interface AdminUserDetail extends AdminUser {
    suspended_by?: string;
    active_sessions: number;
    applications: number;
    experiences: number;
    /** Decrypted only in this single-account detail view, never in the list. */
    email: string;
    /** "" when the account has no school email on file (see backend
     * auth.AdminContactStore.GetAccountContact) — pre-dates the column, or
     * never went through school-email registration. */
    school_email: string;
}

export interface ListUsersFilter {
    status?: AccountStatus;
    identity?: IdentityStatus;
    role?: "admin";
    q?: string;
    cursor?: string;
}

export function listUsers(filter: ListUsersFilter = {}, mfaCode?: string) {
    return apiFetch<Page<AdminUser>>("/api/v1/admin/users", {
        query: { limit: 50, ...filter },
        headers: mfaHeader(mfaCode)
    });
}

export function getUser(accountId: string, mfaCode?: string) {
    return apiFetch<AdminUserDetail>(`/api/v1/admin/users/${accountId}`, {
        headers: mfaHeader(mfaCode)
    });
}

export interface UpdateUserInput {
    username?: string;
    identity_status?: IdentityStatus;
    email?: string;
    school_email?: string;
}

/** Edits the fields the admin panel lets an operator change directly.
 * Password is deliberately never settable here — see resetUserPassword. */
export function updateUser(accountId: string, input: UpdateUserInput, mfaCode?: string) {
    return apiFetch<{ status: string }>(`/api/v1/admin/users/${accountId}`, {
        method: "PUT",
        body: input,
        headers: mfaHeader(mfaCode)
    });
}

/** Re-sends the school-email activation link for an account stuck
 * 'pending_verification' — typically after correcting a mistyped school
 * email via updateUser first. */
export function resendActivation(accountId: string, mfaCode?: string) {
    return apiFetch<{ status: string; expires_at: string }>(
        `/api/v1/admin/users/${accountId}/resend-activation`,
        { method: "POST", headers: mfaHeader(mfaCode) }
    );
}

/** Re-sends the contact-email verification link for an active account whose
 * email is still unverified — separate from resendActivation, which is for
 * the pre-activation school-email link. */
export function resendEmailVerification(accountId: string, mfaCode?: string) {
    return apiFetch<{ status: string; expires_at: string }>(
        `/api/v1/admin/users/${accountId}/resend-email-verification`,
        { method: "POST", headers: mfaHeader(mfaCode) }
    );
}

export function suspendUser(accountId: string, reason: string, mfaCode?: string) {
    return apiFetch<{ status: string; sessions_revoked: number }>(
        `/api/v1/admin/users/${accountId}/suspend`,
        {
            method: "POST",
            body: { reason },
            headers: mfaHeader(mfaCode)
        }
    );
}

export function reinstateUser(accountId: string, reason: string, mfaCode?: string) {
    return apiFetch<{ status: string }>(`/api/v1/admin/users/${accountId}/reinstate`, {
        method: "POST",
        body: { reason },
        headers: mfaHeader(mfaCode)
    });
}

export function forceLogoutUser(accountId: string, reason: string, mfaCode?: string) {
    return apiFetch<{ sessions_revoked: number }>(`/api/v1/admin/users/${accountId}/force-logout`, {
        method: "POST",
        body: { reason },
        headers: mfaHeader(mfaCode)
    });
}

/** Grants or revokes admissions_moderator — a brochure board moderator's
 * scoped access to /admin/admissions only, with no visibility into anything
 * else under /admin (see internal/admissions.IsAdmin and admin-shell.tsx). */
export function setAdmissionsModerator(
    accountId: string,
    grant: boolean,
    reason: string,
    mfaCode?: string
) {
    return apiFetch<{ admissions_moderator: boolean }>(
        `/api/v1/admin/users/${accountId}/admissions-moderator`,
        {
            method: "POST",
            body: { grant, reason },
            headers: mfaHeader(mfaCode)
        }
    );
}

/** Sends the same "set a new password" email the self-service forgot-password
 * flow sends, for a user who's locked out. Never exposes the account's
 * plaintext email to the admin. */
export function resetUserPassword(accountId: string, mfaCode?: string) {
    return apiFetch<{ status: string }>(`/api/v1/admin/users/${accountId}/reset-password`, {
        method: "POST",
        headers: mfaHeader(mfaCode)
    });
}

export interface CreateInvitedUserInput {
    mode: "invite";
    username: string;
    email: string;
}

export interface CreateServiceUserInput {
    mode: "service";
    username: string;
    label: string;
    grant_admin: boolean;
}

/** An active, human-usable account with an admin-chosen password instead of
 * an emailed set-password link — for a throwaway account with no real
 * mailbox behind it (e.g. a batch of brochure-proofreading logins). */
export interface CreatePasswordUserInput {
    mode: "password";
    username: string;
    password: string;
    grant_admissions_moderator: boolean;
}

export interface CreatedInvitedUser {
    account: AdminUser;
    invited: true;
}

export interface CreatedServiceUser {
    account: AdminUser;
    /** Returned once, here, and never recoverable afterward. */
    token: string;
    granted_admin: boolean;
}

export interface CreatedPasswordUser {
    account: AdminUser;
    admissions_moderator: boolean;
}

export async function createUser(
    input: CreateInvitedUserInput | CreateServiceUserInput | CreatePasswordUserInput,
    mfaCode?: string
) {
    const response = await apiFetch<{
        data: CreatedInvitedUser | CreatedServiceUser | CreatedPasswordUser;
    }>("/api/v1/admin/users", {
        method: "POST",
        body: input,
        headers: mfaHeader(mfaCode)
    });
    return response.data;
}

// --- audit log -----------------------------------------------------------

export interface AuditRow {
    id: number;
    actor_account_id?: string;
    action: string;
    entity_type: string;
    entity_key: string;
    before_data?: unknown;
    after_data?: unknown;
    reason: string;
    request_id?: string;
    created_at: string;
}

export interface ListAuditLogFilter {
    entity_type?: string;
    entity_key?: string;
    action?: string;
    actor?: string;
    since?: string;
    until?: string;
    cursor?: string;
}

export function listAuditLog(filter: ListAuditLogFilter = {}, mfaCode?: string) {
    return apiFetch<Page<AuditRow>>("/api/v1/admin/audit-log", {
        query: { limit: 50, ...filter },
        headers: mfaHeader(mfaCode)
    });
}

// --- settings --------------------------------------------------------------

export interface RequireAdminMfaSetting {
    value: boolean;
    is_set?: boolean;
}

/** GET /api/v1/admin/settings/require-admin-mfa */
export function getRequireAdminMfa(mfaCode?: string) {
    return apiFetch<RequireAdminMfaSetting>("/api/v1/admin/settings/require-admin-mfa", {
        headers: mfaHeader(mfaCode)
    });
}

/** POST /api/v1/admin/settings/require-admin-mfa */
export function setRequireAdminMfa(value: boolean, mfaCode?: string) {
    return apiFetch<RequireAdminMfaSetting>("/api/v1/admin/settings/require-admin-mfa", {
        method: "POST",
        body: { value },
        headers: mfaHeader(mfaCode)
    });
}

/** GET /api/v1/admin/settings/mail-reply-template */
export function getMailReplyTemplate(mfaCode?: string) {
    return apiFetch<{ value: string }>("/api/v1/admin/settings/mail-reply-template", {
        headers: mfaHeader(mfaCode)
    });
}

/** POST /api/v1/admin/settings/mail-reply-template — value must contain both [內容] and [簽名]. */
export function setMailReplyTemplate(value: string, mfaCode?: string) {
    return apiFetch<{ value: string }>("/api/v1/admin/settings/mail-reply-template", {
        method: "POST",
        body: { value },
        headers: mfaHeader(mfaCode)
    });
}

/** GET /api/v1/admin/settings/site-policies */
export function getAdminSitePolicies(mfaCode?: string) {
    return apiFetch<{ data: SitePolicyDocuments }>("/api/v1/admin/settings/site-policies", {
        headers: mfaHeader(mfaCode)
    });
}

/** POST /api/v1/admin/settings/site-policies/{policy} */
export function setAdminSitePolicy(policy: SitePolicyKey, value: string, mfaCode?: string) {
    return apiFetch<{ value: string }>("/api/v1/admin/settings/site-policies/" + policy, {
        method: "POST",
        body: { value },
        headers: mfaHeader(mfaCode)
    });
}

// --- forum moderation -----------------------------------------------------

export type ForumThreadStatus = "published" | "hidden" | "locked" | "removed" | "archived";
export type ForumPostStatus = "published" | "hidden" | "removed" | "archived";

/** Moderation view of a thread — includes the poster's account id/username,
 * which the public forum API never exposes. */
export interface AdminForumThread {
    id: string;
    space_id: string;
    title: string;
    status: ForumThreadStatus;
    account_id: string;
    author_username: string;
    created_at: string;
    updated_at: string;
}

export interface AdminForumPost {
    id: string;
    thread_id: string;
    body: string;
    quoted_experience_id?: string;
    status: ForumPostStatus;
    account_id: string;
    author_username: string;
    created_at: string;
}

/** GET /api/v1/admin/forum/threads — every thread across every space, newest first. */
export function adminListForumThreads(cursor?: string, mfaCode?: string) {
    return apiFetch<Page<AdminForumThread>>("/api/v1/admin/forum/threads", {
        query: { limit: 50, cursor },
        headers: mfaHeader(mfaCode)
    });
}

/** GET /api/v1/admin/forum/threads/{id}/posts — every post in a thread, including removed ones. */
export function adminListForumPosts(threadId: string, mfaCode?: string) {
    return apiFetch<{ data: AdminForumPost[] }>(`/api/v1/admin/forum/threads/${threadId}/posts`, {
        headers: mfaHeader(mfaCode)
    });
}

/** POST .../lock — blocks new replies; the thread and its posts stay visible. */
export function lockForumThread(threadId: string, mfaCode?: string) {
    return apiFetch<void>(`/api/v1/admin/forum/threads/${threadId}/lock`, {
        method: "POST",
        headers: mfaHeader(mfaCode)
    });
}

export function unlockForumThread(threadId: string, mfaCode?: string) {
    return apiFetch<void>(`/api/v1/admin/forum/threads/${threadId}/unlock`, {
        method: "POST",
        headers: mfaHeader(mfaCode)
    });
}

/** DELETE .../threads/{id} — soft delete (status becomes 'removed'); hides it from the public forum. */
export function deleteForumThread(threadId: string, mfaCode?: string) {
    return apiFetch<void>(`/api/v1/admin/forum/threads/${threadId}`, {
        method: "DELETE",
        headers: mfaHeader(mfaCode)
    });
}

/** POST .../threads/{id}/archive — keeps a thread as evidence (e.g. a possible
 * legal matter) rather than ordinary moderation; hidden from the public forum
 * like a delete, but tagged separately for admins. Deliberately one-way —
 * there is no unarchive endpoint. */
export function archiveForumThread(threadId: string, mfaCode?: string) {
    return apiFetch<void>(`/api/v1/admin/forum/threads/${threadId}/archive`, {
        method: "POST",
        headers: mfaHeader(mfaCode)
    });
}

/** DELETE .../posts/{id} — soft delete a single reply. */
export function deleteForumPost(postId: string, mfaCode?: string) {
    return apiFetch<void>(`/api/v1/admin/forum/posts/${postId}`, {
        method: "DELETE",
        headers: mfaHeader(mfaCode)
    });
}

/** POST .../posts/{id}/archive — see archiveForumThread. */
export function archiveForumPost(postId: string, mfaCode?: string) {
    return apiFetch<void>(`/api/v1/admin/forum/posts/${postId}/archive`, {
        method: "POST",
        headers: mfaHeader(mfaCode)
    });
}

function mfaHeader(code?: string): Record<string, string> | undefined {
    return code ? { "X-MFA-Code": code } : undefined;
}
