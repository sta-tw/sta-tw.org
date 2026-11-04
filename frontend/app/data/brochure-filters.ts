export type BrochureFilterId = "skills-test" | "interview" | "portfolio";

export type BrochureFilter = {
    id: BrochureFilterId;
    label: string;
    options: Array<{
        label: string;
        value: string;
    }>;
};

// These are the filters supported by the live admissions API view.
export const brochureFilters: BrochureFilter[] = [
    {
        id: "skills-test",
        label: "是否需要術科考試",
        options: [
            { label: "需要術科考試", value: "required" },
            { label: "不需要術科考試", value: "not-required" }
        ]
    },
    {
        id: "interview",
        label: "是否需要面試",
        options: [
            { label: "需要面試", value: "required" },
            { label: "不需要面試", value: "not-required" }
        ]
    },
    {
        id: "portfolio",
        label: "是否需要作品集",
        options: [
            { label: "需要作品集", value: "required" },
            { label: "不需要作品集", value: "not-required" }
        ]
    }
];

/**
 * 開放報名狀態 — planned `registration_status` param on
 * GET /api/v1/admissions/programs/search (MOCK: registration_status, not
 * backed yet). The search response carries no timeline data, and computing
 * it client-side would mean fetching every result's full program and
 * filtering after pagination (wrong counts, half-empty pages), so the
 * search page shows this filter disabled until the backend supports it.
 */
export const registrationStatusFilter = {
    id: "registration-status",
    label: "開放報名狀態",
    options: [
        { label: "開放報名中", value: "open" },
        { label: "即將開放", value: "upcoming" },
        { label: "已截止", value: "closed" }
    ]
} as const;
