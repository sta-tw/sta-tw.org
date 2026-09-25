"use client";

import { createContext, useContext } from "react";
import type { Account } from "../lib/api/types";

export interface AdminContextValue {
    account: Account;
    /** Attach to any /api/v1/admin/* call once the MFA grant window is live;
     * empty when MFA isn't enrolled/required. See lib/api/admin.ts. */
    mfaCode: string;
}

const AdminContext = createContext<AdminContextValue | null>(null);

export const AdminContextProvider = AdminContext.Provider;

/** Only usable inside app/admin — AdminShell guarantees the value is set
 * before any admin page renders. */
export function useAdmin(): AdminContextValue {
    const value = useContext(AdminContext);
    if (!value) {
        throw new Error("useAdmin() called outside the admin shell");
    }
    return value;
}
