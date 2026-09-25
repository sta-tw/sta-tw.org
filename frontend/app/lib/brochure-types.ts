import type { BrochureFilterId } from "../data/brochure-filters";
import type { ExternalLink } from "./open-graph";

export type BrochureFact = {
    label: string;
    value: string;
};

export type BrochureHistory = {
    year: string;
    admitted: string;
    waitlisted: string;
    candidates: string;
    applicants: string;
};

export type BrochureTimelineItem = {
    id: string;
    title: string;
    date: string;
    /** "YYYY-MM-DD", when the underlying data has a real date — powers
     * "新增至日曆". Missing when the item is only a human-readable fallback
     * (e.g. "以官方簡章公告為準"). Also used to color each item relative to
     * today (past/current/next-up/future) — see brochure-detail-tabs.tsx. */
    isoStart?: string;
    isoEnd?: string;
};

/** View model shared by the brochure list and the detail tabs. */
export type Brochure = {
    slug: string;
    university: string;
    department: string;
    group: string;
    title: string;
    summary: string;
    facts: BrochureFact[];
    eligibility: string;
    examFormat: string;
    fee: string;
    timeline: string;
    requirements: Record<BrochureFilterId, "required" | "not-required">;
    externalLinks: ExternalLink[];
    /** The school's/department's own homepage, pinned at the top of "關於學系"
     * — distinct from externalLinks, which comes from brochure_url (the
     * admissions/簡章 info page). Absent when the admin hasn't filled it in. */
    schoolOfficialUrl?: string;
    departmentOfficialUrl?: string;
    registrationTimeline: BrochureTimelineItem[];
    history: BrochureHistory[];
};

export type BrochureSearchFilters = {
    q?: string;
} & Partial<Record<BrochureFilterId, "required" | "not-required">>;
