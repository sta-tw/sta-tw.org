"use client";

import { useCallback, useEffect, useState } from "react";
import { describeError } from "./describe-error";

export type ResourceState<T> =
    | { status: "loading" }
    | { status: "error"; error: unknown }
    | { status: "ready"; data: T };

const LOADING = { status: "loading" } as const;

/** Runs `load` on mount and whenever it changes (pass a stable function —
 * module-level or useCallback); the state reads "loading" again until the
 * new load settles, so stale data from a previous `load` never shows.
 * `setData` patches the loaded value after a mutation without refetching. */
export function useResource<T>(load: (signal: AbortSignal) => Promise<T>) {
    const [version, setVersion] = useState(0);
    // Each result remembers which load/version produced it.
    const [entry, setEntry] = useState<{
        load: typeof load;
        version: number;
        state: ResourceState<T>;
    } | null>(null);

    useEffect(() => {
        const controller = new AbortController();
        load(controller.signal).then(
            (data) => setEntry({ load, version, state: { status: "ready", data } }),
            (error: unknown) => {
                if (!controller.signal.aborted) {
                    setEntry({ load, version, state: { status: "error", error } });
                }
            }
        );
        return () => controller.abort();
    }, [load, version]);

    const state: ResourceState<T> =
        entry && entry.load === load && entry.version === version ? entry.state : LOADING;

    const reload = useCallback(() => setVersion((value) => value + 1), []);

    const setData = useCallback((update: (data: T) => T) => {
        setEntry((current) =>
            current?.state.status === "ready"
                ? { ...current, state: { status: "ready", data: update(current.state.data) } }
                : current
        );
    }, []);

    return { state, reload, setData };
}

/** Busy key + user-facing error for mutations. `run` marks `key` pending
 * (default "action", for forms with a single submit), clears the previous
 * error and resolves true on success. */
export function useAction() {
    const [pending, setPending] = useState<string | null>(null);
    const [error, setError] = useState<string | null>(null);

    const run = useCallback(
        async (
            keyOrAction: string | (() => Promise<unknown>),
            maybeAction?: () => Promise<unknown>
        ) => {
            const [key, action] =
                typeof keyOrAction === "string"
                    ? [keyOrAction, maybeAction!]
                    : ["action", keyOrAction];
            setPending(key);
            setError(null);
            try {
                await action();
                return true;
            } catch (cause) {
                setError(describeError(cause));
                return false;
            } finally {
                setPending(null);
            }
        },
        []
    );

    return { pending, error, setError, run };
}
