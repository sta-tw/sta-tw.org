import { API_BASE_URL, apiFetch } from "./client";

export type OAuthProvider = "google" | "discord";

/** The OAuth providers offered for login and account linking. */
export const oauthProviders: readonly { id: OAuthProvider; label: string }[] = [
    { id: "google", label: "Google" },
    { id: "discord", label: "Discord" }
];

export function oauthProviderLabel(provider: string | null | undefined) {
    return oauthProviders.find((item) => item.id === provider)?.label;
}

/** Message for the `?oauth_error=<code>` the backend redirects back with,
 * after a login or a bind round trip. */
export function oauthErrorMessage(code: string, provider?: string | null): string {
    const label = oauthProviderLabel(provider) ?? "第三方";
    switch (code) {
        case "oauth_not_bound":
            return `這個 ${label} 帳號尚未連動 S.T.A 帳號，請先以帳號密碼登入，到「帳號驗證狀態」連動。`;
        case "oauth_conflict":
            return `這個 ${label} 帳號已經連動到其他 S.T.A 帳號。`;
        case "oauth_unavailable":
            return "目前無法連線到第三方登入服務，請稍後再試。";
        default:
            return "第三方帳號驗證失敗，請再試一次。";
    }
}

/** GET /api/v1/auth/oauth/{provider}/start — a plain link (full-page
 * navigation) that signs in and lands back on `next`. */
export function oauthLoginUrl(provider: OAuthProvider, next: string) {
    return `${API_BASE_URL}/api/v1/auth/oauth/${encodeURIComponent(provider)}/start?return_to=${encodeURIComponent(next)}`;
}

/** POST /api/v1/auth/oauth/{provider}/bind/start — send the browser to the
 * returned authorization_url. The backend lands it back on `returnTo` (a
 * same-origin path) with `oauth=success` appended. */
export function startOAuthBind(provider: OAuthProvider, returnTo: string) {
    return apiFetch<{ authorization_url: string }>(
        `/api/v1/auth/oauth/${encodeURIComponent(provider)}/bind/start`,
        { method: "POST", query: { return_to: returnTo } }
    );
}

export interface LinkedIdentity {
    provider: OAuthProvider;
    linked_at: string;
    /** Provider-side name to show, e.g. a Discord handle or Google email. */
    display?: string;
}

/** GET /api/v1/auth/identities — exposes provider and link time only. */
export function listLinkedIdentities(options?: { signal?: AbortSignal }) {
    return apiFetch<{ data: LinkedIdentity[] }>("/api/v1/auth/identities", {
        signal: options?.signal
    });
}
