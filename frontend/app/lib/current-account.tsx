"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { getCurrentAccount, type MeResult } from "./api/auth";
import { getMyProfile, type Profile } from "./api/profile";
import { subscribeAuthChanged } from "./auth-events";

export type CurrentAccountState = (
    | { status: "loading"; me: null }
    | { status: "anonymous"; me: null }
    | { status: "authenticated"; me: MeResult }
) & {
    /** The signed-in user's profile; null while loading, signed out, or
     * after profileError. */
    profile: Profile | null;
    profileError: unknown;
    /** Replace the cached profile after a successful edit (no refetch). */
    setProfile: (profile: Profile) => void;
    reloadProfile: () => void;
};

const CurrentAccountContext = createContext<CurrentAccountState | null>(null);

type Session =
    | { status: "loading" | "anonymous"; me: null }
    | { status: "authenticated"; me: MeResult };

type ProfileSlot = { profile: Profile | null; error: unknown };

/** Loads GET /auth/me once for the whole site (refetched whenever
 * notifyAuthChanged() fires after login/logout), then the signed-in user's
 * GET /profile so the navbar and /account share one copy. */
export function CurrentAccountProvider({ children }: { children: React.ReactNode }) {
    const [session, setSession] = useState<Session>({ status: "loading", me: null });
    const [slot, setSlot] = useState<ProfileSlot>({ profile: null, error: null });
    const [profileRequest, setProfileRequest] = useState(0);

    useEffect(() => {
        let cancelled = false;
        function refresh() {
            getCurrentAccount()
                .then((me) => !cancelled && setSession({ status: "authenticated", me }))
                .catch(() => !cancelled && setSession({ status: "anonymous", me: null }));
        }
        refresh();
        const unsubscribe = subscribeAuthChanged(refresh);
        return () => {
            cancelled = true;
            unsubscribe();
        };
    }, []);

    const accountId = session.me?.account.id;
    useEffect(() => {
        if (!accountId) return;
        const controller = new AbortController();
        getMyProfile({ signal: controller.signal }).then(
            ({ data }) => setSlot({ profile: data, error: null }),
            (error: unknown) => {
                if (!controller.signal.aborted) setSlot({ profile: null, error });
            }
        );
        return () => {
            controller.abort();
            setSlot({ profile: null, error: null });
        };
    }, [accountId, profileRequest]);

    const setProfile = useCallback((profile: Profile) => setSlot({ profile, error: null }), []);
    const reloadProfile = useCallback(() => setProfileRequest((value) => value + 1), []);

    const value = useMemo(
        () =>
            ({
                ...session,
                profile: slot.profile,
                profileError: slot.error,
                setProfile,
                reloadProfile
            }) as CurrentAccountState,
        [session, slot, setProfile, reloadProfile]
    );

    return (
        <CurrentAccountContext.Provider value={value}>{children}</CurrentAccountContext.Provider>
    );
}

export function useCurrentAccount(): CurrentAccountState {
    const value = useContext(CurrentAccountContext);
    if (!value) throw new Error("useCurrentAccount must be used inside CurrentAccountProvider");
    return value;
}
