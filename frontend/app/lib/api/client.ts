import { ApiError } from "./types";

/**
 * Base URL of the Go backend. Set NEXT_PUBLIC_API_BASE_URL in .env.local for
 * local dev (defaults to the docker-compose stack's default port). This is a
 * client-only value (the app is a static export; every call happens in the
 * browser), so it's fine that it's baked in at build time.
 */
export const API_BASE_URL = (
    process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080"
).replace(/\/$/, "");

const CSRF_COOKIE = "sta_csrf";
const CSRF_HEADER = "X-CSRF-Token";

function readCookie(name: string): string | null {
    if (typeof document === "undefined") return null;
    const match = document.cookie.match(new RegExp(`(?:^|; )${name}=([^;]*)`));
    return match ? decodeURIComponent(match[1]) : null;
}

type Method = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

export interface ApiFetchOptions {
    method?: Method;
    /** JSON-serialised as the request body. Omit for a bodyless request. */
    body?: unknown;
    /** Extra query params appended to the path. */
    query?: Record<string, string | number | boolean | undefined>;
    /** Extra request headers, e.g. X-MFA-Code for the admin surface. */
    headers?: Record<string, string>;
    signal?: AbortSignal;
}

function withQuery(path: string, query?: ApiFetchOptions["query"]): string {
    if (!query) return path;
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) {
        if (value !== undefined) params.set(key, String(value));
    }
    const qs = params.toString();
    return qs ? `${path}${path.includes("?") ? "&" : "?"}${qs}` : path;
}

/**
 * Calls the backend at `path` (e.g. "/api/v1/auth/me"), sending the session
 * cookie and — for a mutating method — the X-CSRF-Token header the backend
 * requires alongside it (see docs/api-migration-from-workers.md).
 *
 * Resolves with the parsed JSON body (or undefined for a 204), and rejects
 * with ApiError for any non-2xx response.
 */
export async function apiFetch<T = unknown>(
    path: string,
    options: ApiFetchOptions = {}
): Promise<T> {
    const method = options.method ?? "GET";
    const headers: Record<string, string> = { Accept: "application/json" };
    let body: string | undefined;

    if (options.body !== undefined) {
        headers["Content-Type"] = "application/json";
        body = JSON.stringify(options.body);
    }
    if (method !== "GET") {
        const csrf = readCookie(CSRF_COOKIE);
        if (csrf) headers[CSRF_HEADER] = csrf;
    }
    if (options.headers) {
        Object.assign(headers, options.headers);
    }

    let response: Response;
    try {
        response = await fetch(`${API_BASE_URL}${withQuery(path, options.query)}`, {
            method,
            headers,
            body,
            credentials: "include",
            signal: options.signal
        });
    } catch {
        throw new ApiError(0, "network_error", "無法連線到後端服務，請確認服務是否啟動。");
    }

    return parseResponse<T>(response);
}

/**
 * Sends a multipart/form-data request without setting Content-Type manually.
 * The browser must add the multipart boundary itself, which is why uploads
 * use this helper instead of apiFetch's JSON body option.
 */
export async function apiUpload<T = unknown>(
    path: string,
    formData: FormData,
    options: Omit<ApiFetchOptions, "body"> = {}
): Promise<T> {
    const method = options.method ?? "POST";
    const headers: Record<string, string> = { Accept: "application/json" };

    if (method !== "GET") {
        const csrf = readCookie(CSRF_COOKIE);
        if (csrf) headers[CSRF_HEADER] = csrf;
    }
    if (options.headers) {
        Object.assign(headers, options.headers);
    }

    let response: Response;
    try {
        response = await fetch(`${API_BASE_URL}${withQuery(path, options.query)}`, {
            method,
            headers,
            body: formData,
            credentials: "include",
            signal: options.signal
        });
    } catch {
        throw new ApiError(0, "network_error", "無法連線到後端服務，請確認服務是否啟動。");
    }

    return parseResponse<T>(response);
}

async function parseResponse<T>(response: Response): Promise<T> {
    if (response.status === 204) {
        return undefined as T;
    }

    const raw = await response.text();
    const data = raw ? safeJsonParse(raw) : undefined;

    if (!response.ok) {
        const errorBody = data as { error?: { code?: string; message?: string } } | undefined;
        throw new ApiError(
            response.status,
            errorBody?.error?.code ?? "unknown_error",
            errorBody?.error?.message ?? response.statusText
        );
    }
    return data as T;
}

function safeJsonParse(raw: string): unknown {
    try {
        return JSON.parse(raw);
    } catch {
        return undefined;
    }
}
