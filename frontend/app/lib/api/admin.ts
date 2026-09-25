import { apiFetch } from "./client";
import type { Page } from "./forum";

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

export type AccountStatus = "active" | "suspended" | "deleted";
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

function mfaHeader(code?: string): Record<string, string> | undefined {
    return code ? { "X-MFA-Code": code } : undefined;
}
