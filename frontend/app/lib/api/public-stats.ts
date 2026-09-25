import { apiFetch } from "./client";

export interface PublicStats {
    brochure_count: number;
    registered_accounts: number;
}

export function getPublicStats(signal?: AbortSignal): Promise<{ data: PublicStats }> {
    return apiFetch<{ data: PublicStats }>("/api/v1/stats/public", { signal });
}
