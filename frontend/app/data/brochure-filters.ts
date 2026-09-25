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
