import type { ReactNode } from "react";
import type { BrochureFilterId } from "../data/brochure-filters";
import type { ExternalLink } from "./open-graph";

export type BrochureFact = {
    label: string;
    value: ReactNode;
};

export type BrochureHistory = {
    year: string;
    quota: string;
    applicants: string;
    admitted: string;
    waitlisted: string;
    promoted: string;
    notes: string;
};

export type BrochureTimelineItem = {
    id: string;
    title: string;
    date: string;
    notes?: string;
    /** Calendar date in YYYY-MM-DD. For a deadline-only row stored in
     * end_date, isoStart is normalized to that date so the event is selectable
     * as a one-day event. Missing when there is no usable date. Also used to
     * color each item relative to today — see brochure-detail-tabs.tsx. */
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
