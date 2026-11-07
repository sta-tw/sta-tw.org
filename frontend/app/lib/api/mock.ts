/**
 * Frontend-only stand-ins for endpoints the backend doesn't expose yet.
 *
 * Every mocked client function must:
 *  - keep the exact signature/return envelope the real endpoint will have
 *    (document it as `MOCK: <METHOD> <path>` in its JSDoc), so switching to
 *    apiFetch later is a one-line change per function;
 *  - persist through mockStore so the UI behaves realistically across
 *    reloads in the same browser.
 *
 * Real endpoints that only lack a field or two (e.g. experience `tags`) are
 * split in two: the real request stays a private `real*` function above the
 * `// Not backed yet` divider, and the exported function under the public
 * name wraps it below the divider, tagged `MOCK:`. Call sites import the
 * public name only, so retiring a mock means deleting the wrapper block and
 * renaming `realX` back to `X` in that one file. Wrappers do their storage
 * bookkeeping synchronously after the real call — never add mockResponse
 * latency on top of a real request.
 *
 * grep "MOCK:" to find everything that still needs a real backend.
 */

const STORAGE_PREFIX = "sta:mock:";
const LATENCY_MS = 250;

function read<T>(key: string, fallback: T): T {
    if (typeof window === "undefined") return fallback;
    try {
        const raw = window.localStorage.getItem(STORAGE_PREFIX + key);
        return raw ? (JSON.parse(raw) as T) : fallback;
    } catch {
        return fallback;
    }
}

function write<T>(key: string, value: T) {
    if (typeof window === "undefined") return;
    try {
        window.localStorage.setItem(STORAGE_PREFIX + key, JSON.stringify(value));
    } catch {
        // Storage can be unavailable (private mode / quota); the mock then
        // simply doesn't persist, which is fine for a stand-in.
    }
}

export const mockStore = {
    read,
    write
};

/** Runs `produce` after a short delay so loading states are exercised;
 * a thrown error becomes a rejection, like a failed request, and an abort
 * rejects like apiFetch does. */
export function mockResponse<T>(produce: () => T | Promise<T>, signal?: AbortSignal): Promise<T> {
    return new Promise((resolve, reject) => {
        const timer = setTimeout(() => {
            signal?.removeEventListener("abort", abort);
            Promise.resolve().then(produce).then(resolve, reject);
        }, LATENCY_MS);
        function abort() {
            clearTimeout(timer);
            reject(new DOMException("Aborted", "AbortError"));
        }
        if (signal?.aborted) abort();
        else signal?.addEventListener("abort", abort, { once: true });
    });
}
