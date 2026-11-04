"use client";

import type { Profile } from "../lib/api/profile";
import { useCurrentAccount } from "../lib/current-account";
import type { ResourceState } from "../lib/use-resource";

/** The signed-in account for pages under /account (AccountShell only
 * renders them once /auth/me says so). `profileState` feeds <Resource> for
 * pages that need the profile before rendering. */
export function useAccount() {
    const account = useCurrentAccount();
    if (account.status !== "authenticated") {
        throw new Error("useAccount must be used inside /account");
    }
    const profileState: ResourceState<Profile> = account.profile
        ? { status: "ready", data: account.profile }
        : account.profileError
          ? { status: "error", error: account.profileError }
          : { status: "loading" };
    return { ...account, profileState };
}
