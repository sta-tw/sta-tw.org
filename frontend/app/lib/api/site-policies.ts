import { apiFetch } from "./client";

export type SitePolicyKey = "terms" | "privacy";

export interface SitePolicyDocument {
    value: string;
    updated_at?: string;
}

export interface SitePolicyDocuments {
    terms: SitePolicyDocument;
    privacy: SitePolicyDocument;
}

/** GET /api/v1/site-policies — public, uncached policy content. */
export function getSitePolicies() {
    return apiFetch<{ data: SitePolicyDocuments }>("/api/v1/site-policies");
}
