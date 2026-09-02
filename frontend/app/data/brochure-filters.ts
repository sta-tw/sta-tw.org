export type BrochureFilter = {
    id: "skills-test" | "interview" | "portfolio";
    label: string;
    options: Array<{
        label: string;
        value: string;
    }>;
};

// This static shape mirrors the query parameters expected by the future brochure API.
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
