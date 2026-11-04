/** Only same-origin relative paths are honoured as `?next=`, so the login
 * page can't be used as an open redirect ("//evil.com" and "/\evil.com" are
 * protocol-relative). */
export function safeNextPath(value: string | null): string {
    if (!value || !value.startsWith("/") || value.startsWith("//") || value.startsWith("/\\")) {
        return "/";
    }
    return value;
}

/** /login that returns to `next` after signing in. */
export function loginHref(next: string) {
    return `/login/?next=${encodeURIComponent(next)}`;
}
