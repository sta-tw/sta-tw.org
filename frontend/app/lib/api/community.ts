import { apiFetch } from "./client";

export interface DiscordCommunityStats {
    member_count: number;
    approximate_member_count: number;
}

export function getDiscordCommunityStats(signal?: AbortSignal): Promise<DiscordCommunityStats> {
    return apiFetch<DiscordCommunityStats>("/api/v1/community/discord", { signal });
}
