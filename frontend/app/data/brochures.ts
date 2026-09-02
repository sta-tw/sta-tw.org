import type { BrochureFilterId } from "./brochure-filters";
import type { ExternalLink } from "../lib/open-graph";

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
    registration: BrochureFact[];
    history: BrochureHistory[];
};

export type BrochureSearchFilters = {
    q?: string;
} & Partial<Record<BrochureFilterId, "required" | "not-required">>;

// Static fixtures keep the UI usable now and mirror the response shape of the future brochure API.
// All records below are illustrative only and must be replaced with verified API data before release.
export const brochures: Brochure[] = [
    {
        slug: "nycu-information-engineering-a",
        university: "國立陽明交通大學",
        department: "資訊工程學系",
        group: "A 組",
        title: "國立陽明交通大學 / 資訊工程學系（A 組）",
        summary: "歡迎對資訊工程領域懷抱高度興趣、具備相關潛力或專長之優秀考生報名本組招生。",
        facts: [
            { label: "學群", value: "資訊學群、工程學群" },
            { label: "學類", value: "資訊工程學類" },
            { label: "招生人數", value: "10 人" }
        ],
        eligibility: "歡迎對資訊工程領域懷抱高度興趣、具備相關潛力或專長之優秀考生報名本組招生。",
        examFormat: "初試－書面審查、複試－面試",
        fee: "900 元、通過初試參加複試者另收 400 元",
        timeline: "詳見「報名資訊」頁面，手機版介面請右滑。",
        requirements: {
            "skills-test": "not-required",
            interview: "required",
            portfolio: "not-required"
        },
        externalLinks: [
            {
                url: "https://www.cs.nycu.edu.tw/intro?locale=zh",
                fallbackTitle: "國立陽明交通大學資訊工程學系｜系所介紹"
            },
            {
                url: "https://www.cs.nycu.edu.tw/education/undergraduate?locale=zh",
                fallbackTitle: "國立陽明交通大學資訊工程學系｜學士班課程與修業"
            },
            {
                url: "https://www.cs.nycu.edu.tw/admission/undergraduate?locale=zh",
                fallbackTitle: "國立陽明交通大學資訊工程學系｜招生資訊"
            }
        ],
        registration: [
            { label: "報名期間", value: "待簡章公告" },
            { label: "放榜日期", value: "待簡章公告" },
            { label: "報考校系限制", value: "依當年度簡章規定" },
            { label: "其他限制", value: "依當年度簡章規定" }
        ],
        history: [
            { year: "115", admitted: "2", waitlisted: "2", candidates: "40", applicants: "－" },
            { year: "114", admitted: "2", waitlisted: "1", candidates: "30", applicants: "－" },
            { year: "113", admitted: "1", waitlisted: "0", candidates: "20", applicants: "－" },
            { year: "112", admitted: "1", waitlisted: "無資料", candidates: "10", applicants: "－" }
        ]
    },
    {
        slug: "demo-taipei-university-information-management-b",
        university: "臺北示範大學",
        department: "資訊管理學系",
        group: "B 組",
        title: "臺北示範大學 / 資訊管理學系（B 組）",
        summary: "尋找能以科技回應真實問題、並願意持續探索資料與數位服務的學生。",
        facts: [
            { label: "學群", value: "資訊學群、管理學群" },
            { label: "學類", value: "資訊管理學類" },
            { label: "招生人數", value: "6 人" }
        ],
        eligibility:
            "具備數位工具應用、資料分析、社群經營或專題實作經驗，並能清楚說明自己的學習歷程。",
        examFormat: "初試－書面審查、複試－面試",
        fee: "850 元、通過初試參加複試者另收 300 元",
        timeline: "報名與複試日期請以當年度招生簡章為準。",
        requirements: {
            "skills-test": "not-required",
            interview: "required",
            portfolio: "not-required"
        },
        externalLinks: [],
        registration: [
            { label: "報名期間", value: "待簡章公告" },
            { label: "放榜日期", value: "待簡章公告" },
            { label: "報考校系限制", value: "依當年度簡章規定" },
            { label: "其他限制", value: "須完成線上報名程序" }
        ],
        history: [
            { year: "115", admitted: "2", waitlisted: "1", candidates: "28", applicants: "－" },
            { year: "114", admitted: "2", waitlisted: "2", candidates: "25", applicants: "－" },
            { year: "113", admitted: "1", waitlisted: "1", candidates: "22", applicants: "－" },
            { year: "112", admitted: "1", waitlisted: "0", candidates: "18", applicants: "－" }
        ]
    },
    {
        slug: "demo-arts-university-visual-communication-a",
        university: "臺灣藝術創新大學",
        department: "視覺傳達設計學系",
        group: "A 組",
        title: "臺灣藝術創新大學 / 視覺傳達設計學系（A 組）",
        summary: "重視創作觀點與表達能力，期待看見你如何透過視覺設計回應生活與社會。",
        facts: [
            { label: "學群", value: "藝術學群、建築設計學群" },
            { label: "學類", value: "視覺傳達設計學類" },
            { label: "招生人數", value: "8 人" }
        ],
        eligibility:
            "具備平面、影像、互動設計、插畫或其他視覺創作經驗，並可提供能展現創作脈絡的作品集。",
        examFormat: "初試－作品集與書面審查、複試－術科與面試",
        fee: "1,000 元、通過初試參加複試者另收 500 元",
        timeline: "報名與複試日期請以當年度招生簡章為準。",
        requirements: {
            "skills-test": "required",
            interview: "required",
            portfolio: "required"
        },
        externalLinks: [],
        registration: [
            { label: "報名期間", value: "待簡章公告" },
            { label: "放榜日期", value: "待簡章公告" },
            { label: "報考校系限制", value: "依當年度簡章規定" },
            { label: "其他限制", value: "複試須攜帶原作或指定材料" }
        ],
        history: [
            { year: "115", admitted: "3", waitlisted: "2", candidates: "36", applicants: "－" },
            { year: "114", admitted: "3", waitlisted: "1", candidates: "34", applicants: "－" },
            { year: "113", admitted: "2", waitlisted: "2", candidates: "31", applicants: "－" },
            { year: "112", admitted: "2", waitlisted: "1", candidates: "29", applicants: "－" }
        ]
    },
    {
        slug: "demo-humanities-university-english-a",
        university: "中央人文大學",
        department: "英國語文學系",
        group: "A 組",
        title: "中央人文大學 / 英國語文學系（A 組）",
        summary: "歡迎對語言、文學、翻譯或跨文化交流有長期投入，並能展現自主學習能力的學生。",
        facts: [
            { label: "學群", value: "外語學群、文史哲學群" },
            { label: "學類", value: "英語文學類" },
            { label: "招生人數", value: "5 人" }
        ],
        eligibility:
            "具備語言學習、閱讀研究、翻譯、國際交流或相關社群參與經驗，並能提出具體學習成果。",
        examFormat: "初試－書面審查",
        fee: "800 元",
        timeline: "報名與放榜日期請以當年度招生簡章為準。",
        requirements: {
            "skills-test": "not-required",
            interview: "not-required",
            portfolio: "not-required"
        },
        externalLinks: [],
        registration: [
            { label: "報名期間", value: "待簡章公告" },
            { label: "放榜日期", value: "待簡章公告" },
            { label: "報考校系限制", value: "依當年度簡章規定" },
            { label: "其他限制", value: "無須複試" }
        ],
        history: [
            { year: "115", admitted: "2", waitlisted: "1", candidates: "19", applicants: "－" },
            { year: "114", admitted: "2", waitlisted: "0", candidates: "17", applicants: "－" },
            { year: "113", admitted: "1", waitlisted: "1", candidates: "15", applicants: "－" },
            { year: "112", admitted: "1", waitlisted: "0", candidates: "13", applicants: "－" }
        ]
    }
];

export function getBrochure(slug: string) {
    return brochures.find((brochure) => brochure.slug === slug);
}

export function searchBrochures(filters: BrochureSearchFilters) {
    const query = filters.q?.trim().toLocaleLowerCase();

    return brochures.filter((brochure) => {
        const searchableText = [
            brochure.title,
            brochure.university,
            brochure.department,
            brochure.group,
            brochure.summary,
            ...brochure.facts.flatMap((fact) => [fact.label, fact.value])
        ]
            .join(" ")
            .toLocaleLowerCase();

        const matchesQuery = !query || searchableText.includes(query);
        const matchesRequirements = (
            Object.keys(brochure.requirements) as BrochureFilterId[]
        ).every(
            (filterId) =>
                !filters[filterId] || filters[filterId] === brochure.requirements[filterId]
        );

        return matchesQuery && matchesRequirements;
    });
}
