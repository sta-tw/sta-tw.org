import { API_BASE_URL, apiFetch } from "./client";
import { mockResponse, mockStore } from "./mock";

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

// ---------------------------------------------------------------------------
// Not backed yet — see lib/api/mock.ts.

interface LinkedIdentity {
    provider: OAuthProvider;
    linked_at: string;
    /** Provider-side name to show, e.g. a Discord handle or Google email. */
    display?: string;
}

const IDENTITIES_KEY = "oauth-identities";

/** Stands in for the backend recording a binding: if the current URL is
 * the return leg of a successful bind (`?oauth=success&provider=…`, see
 * startOAuthBind), store that provider. Read synchronously so it happens
 * before the page strips the query. */
function recordReturnedBinding(current: LinkedIdentity[]) {
    if (typeof window === "undefined") return current;
    const params = new URLSearchParams(window.location.search);
    const provider = params.get("provider");
    if (params.get("oauth") !== "success" || !oauthProviderLabel(provider)) return current;
    if (current.some((identity) => identity.provider === provider)) return current;
    const next = [
        ...current,
        { provider: provider as OAuthProvider, linked_at: new Date().toISOString() }
    ];
    mockStore.write<LinkedIdentity[]>(IDENTITIES_KEY, next);
    return next;
}

/** MOCK: GET /api/v1/auth/identities */
export function listLinkedIdentities(options?: { signal?: AbortSignal }) {
    const data = recordReturnedBinding(mockStore.read<LinkedIdentity[]>(IDENTITIES_KEY, []));
    return mockResponse(() => ({ data }), options?.signal);
}
