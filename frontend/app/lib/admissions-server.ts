import type { AdmissionProgram } from "./api/admissions";

const API_BASE_URL = (
    process.env.STA_INTERNAL_API_BASE_URL ||
    process.env.NEXT_PUBLIC_API_BASE_URL ||
    "http://localhost:8080"
).replace(/\/$/, "");

/** Fetch a fresh public program record for request-time page rendering. */
export async function getProgramForPage(identifier: string): Promise<AdmissionProgram | null> {
    const response = await fetch(
        `${API_BASE_URL}/api/v1/admissions/programs/${encodeURIComponent(identifier)}`,
        {
            headers: { Accept: "application/json" },
            cache: "no-store"
        }
    );

    if (response.status === 400 || response.status === 404) return null;
    if (!response.ok) {
        throw new Error(`Failed to load admission program (${response.status})`);
    }

    const payload = (await response.json()) as { data: AdmissionProgram };
    return payload.data;
}
