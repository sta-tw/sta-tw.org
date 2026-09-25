const AUTH_CHANGED_EVENT = "sta:auth-changed";

/** Call after login/logout succeeds so already-mounted client components
 * (e.g. the navbar, which only fetches /auth/me once on mount) know to
 * refetch the current account instead of waiting for a full page reload. */
export function notifyAuthChanged() {
    if (typeof window !== "undefined") {
        window.dispatchEvent(new Event(AUTH_CHANGED_EVENT));
    }
}

export function subscribeAuthChanged(callback: () => void) {
    if (typeof window === "undefined") return () => {};
    window.addEventListener(AUTH_CHANGED_EVENT, callback);
    return () => window.removeEventListener(AUTH_CHANGED_EVENT, callback);
}
