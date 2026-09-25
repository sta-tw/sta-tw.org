import { apiFetch } from "./client";
import type { Account } from "./types";

export interface RegisterInput {
    username: string;
    /** Contact email — any domain. */
    email: string;
    /** Must be a *.edu.tw address. The account stays inactive until the
     * password-set link mailed here is used (see confirmPasswordReset). */
    school_email: string;
}

export interface RegisterResult {
    account: Account;
}

/** POST /api/v1/auth/register — no password field: the account is created
 * 'pending_verification' and a password-set link is emailed to school_email.
 * There's nothing to log in with until that link is used. */
export function registerAccount(input: RegisterInput) {
    return apiFetch<RegisterResult>("/api/v1/auth/register", { method: "POST", body: input });
}

/** POST /api/v1/auth/password-reset/confirm — sets a new password from a
 * reset/activation token (see /reset-password). For a school-email
 * registration token this also activates the account. */
export function confirmPasswordReset(token: string, newPassword: string) {
    return apiFetch<void>("/api/v1/auth/password-reset/confirm", {
        method: "POST",
        body: { token, new_password: newPassword }
    });
}

export interface LoginInput {
    username: string;
    password: string;
}

export interface LoginResult {
    account: Account;
    expires_at: string;
}

/** POST /api/v1/auth/login — by username only, not email. Sets the session
 * (HttpOnly) and CSRF cookies. */
export function login(input: LoginInput) {
    return apiFetch<LoginResult>("/api/v1/auth/login", { method: "POST", body: input });
}

/** POST /api/v1/auth/logout — clears the session for the current device. */
export function logout() {
    return apiFetch<void>("/api/v1/auth/logout", { method: "POST" });
}

export interface MeResult {
    account: Account;
    /** Cheap to trust: /auth/me computes this from account_roles directly,
     * unlike probing an actual /api/v1/admin/* endpoint (which would also
     * trigger the admin-MFA gate as a side effect). */
    is_admin: boolean;
    /** True for a full admin too, but also true for an account holding only
     * the narrower admissions_moderator role (a brochure board moderator) —
     * someone who should see a way into /admin/admissions without
     * qualifying for is_admin or anything else under /admin. */
    can_manage_admissions: boolean;
}

/** GET /api/v1/auth/me — rejects with ApiError(401) when not logged in. */
export function getCurrentAccount() {
    return apiFetch<MeResult>("/api/v1/auth/me");
}

export interface AdminMfaStatus {
    enabled: boolean;
    required: boolean;
}

/** GET /api/v1/auth/admin-mfa/status — whether the caller has TOTP enrolled
 * and whether the deployment requires it for /admin/* at all. */
export function getAdminMfaStatus() {
    return apiFetch<AdminMfaStatus>("/api/v1/auth/admin-mfa/status");
}

export interface VerifyAdminMfaResult {
    verified: boolean;
    expires_at: string;
    grant_seconds: number;
}

/** POST /api/v1/auth/admin-mfa/verify — checks a TOTP code and opens the grant
 * window (STA_ADMIN_MFA_GRANT_TTL) during which /admin/* calls need no
 * X-MFA-Code header. */
export function verifyAdminMfa(code: string) {
    return apiFetch<VerifyAdminMfaResult>("/api/v1/auth/admin-mfa/verify", {
        method: "POST",
        body: { code }
    });
}
