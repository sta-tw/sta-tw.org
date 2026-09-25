import type { BrochureFilterId } from "../data/brochure-filters";
import type { Brochure, BrochureHistory, BrochureTimelineItem } from "./brochure-types";
import type { ExternalLink, OpenGraphPreview } from "./open-graph";
import type { AdmissionExamItem, AdmissionProgram, AdmissionTimelineEvent } from "./api/admissions";

const MISSING_VALUE = "-";

function hasValue(value: string | undefined) {
    return Boolean(value && value.trim() && value.trim() !== MISSING_VALUE);
}

function compactText(value: string) {
    return value.replace(/\s+/g, "").toLocaleLowerCase();
}

function includesAny(value: string, keywords: string[]) {
    const compact = compactText(value);
    return keywords.some((keyword) => compact.includes(compactText(keyword)));
}

function formatDate(value: string) {
    if (!hasValue(value)) return "以官方簡章公告為準";
    const parsed = new Date(`${value}T00:00:00`);
    if (Number.isNaN(parsed.getTime())) return value;
    return new Intl.DateTimeFormat("zh-TW", {
        year: "numeric",
        month: "numeric",
        day: "numeric"
    }).format(parsed);
}

function programGroup(name: string) {
    const match = name.match(/[（(]([^（）()]+)[）)]\s*$/);
    if (!match) return { department: name, group: "" };
    return {
        department: name.slice(0, match.index).trim(),
        group: match[1].trim()
    };
}

function requirementStatus(
    items: AdmissionExamItem[]
): Record<BrochureFilterId, "required" | "not-required"> {
    const examText = items.map((item) => `${item.name} ${item.description}`).join(" ");
    return {
        "skills-test": includesAny(examText, ["術科", "實作", "術科考試"])
            ? "required"
            : "not-required",
        interview: includesAny(examText, ["面試", "口試"]) ? "required" : "not-required",
        portfolio: includesAny(examText, ["作品集", "備審", "學習歷程", "學習成果"])
            ? "required"
            : "not-required"
    };
}

// Blank end_date = point-in-time; set end_date = range (same start/end date = same-day window).
function formatTimelineEventDate(event: AdmissionTimelineEvent): string {
    if (!hasValue(event.start_date)) {
        return "以官方簡章公告為準";
    }
    const startDatePart = formatDate(event.start_date);
    const startTimePart = hasValue(event.start_time) ? event.start_time : "";
    const spans = hasValue(event.end_date);
    if (!spans) {
        return startTimePart ? `${startDatePart} ${startTimePart}` : startDatePart;
    }
    const startLabel = startTimePart ? `${startDatePart} ${startTimePart}` : startDatePart;
    if (event.end_date === event.start_date) {
        // Same-day window: skip repeating the date on the end side.
        return hasValue(event.end_time) ? `${startLabel} ～ ${event.end_time}` : startLabel;
    }
    const endDatePart = formatDate(event.end_date);
    const endLabel = hasValue(event.end_time) ? `${endDatePart} ${event.end_time}` : endDatePart;
    return `${startLabel} ～ ${endLabel}`;
}

// timeline_events (招生時程) is the single source for every schedule milestone.
function registrationTimeline(program: AdmissionProgram): BrochureTimelineItem[] {
    if (program.timeline_events.length > 0) {
        return program.timeline_events.map((event) => ({
            id: `timeline-${event.sort_order}`,
            title: event.name,
            date: formatTimelineEventDate(event),
            isoStart: hasValue(event.start_date) ? event.start_date : undefined,
            isoEnd: hasValue(event.end_date)
                ? event.end_date
                : hasValue(event.start_date)
                  ? event.start_date
                  : undefined
        }));
    }
    return [
        {
            id: "pending",
            title: "招生時程",
            date: "以官方簡章公告為準"
        }
    ];
}

// school_code+program_code stays stable across yearly 大表 re-imports, so this is
// just every academic_year row for that identity; years with no counts are dropped.
export function programHistoryFromYears(years: AdmissionProgram[]): BrochureHistory[] {
    return years
        .filter((year) =>
            [year.applicant_count, year.admitted_count, year.waitlisted_count].some(hasValue)
        )
        .map((year) => ({
            year: `${year.academic_year}`,
            admitted: hasValue(year.admitted_count) ? year.admitted_count : "-",
            // waitlisted_count is 大表's 備取人數, not 遞補人數 — maps to candidates below.
            waitlisted: "-",
            candidates: hasValue(year.waitlisted_count) ? year.waitlisted_count : "-",
            applicants: hasValue(year.applicant_count) ? year.applicant_count : "-"
        }));
}

export function admissionProgramToBrochure(program: AdmissionProgram): Brochure {
    const { department, group } = programGroup(program.admission_program_name);
    const examNames = program.exam_items.map((item) => item.name).filter(Boolean);
    const eligibility = [
        program.special_talent_target,
        program.different_education_backgrounds,
        program.different_education_other
    ]
        .filter(hasValue)
        .join("；");
    // brochure_url is usually the admissions website, not a PDF — only label it
    // "官方招生簡章" when it actually is one.
    const brochureUrlIsPdf = /\.pdf(?:[?#].*)?$/i.test(program.brochure_url);
    const externalLinks: ExternalLink[] = hasValue(program.brochure_url)
        ? [
              {
                  url: program.brochure_url,
                  fallbackTitle: brochureUrlIsPdf ? "官方招生簡章" : "學系官方網站"
              }
          ]
        : [];
    const scheduleItems = registrationTimeline(program);

    return {
        slug: program.program_identifier,
        university: program.school_name,
        department,
        group,
        title: `${program.school_name} / ${program.admission_program_name}`,
        summary: hasValue(program.special_talent_target)
            ? program.special_talent_target
            : "請參閱當年度官方招生資料與簡章。",
        facts: [
            { label: "學年度", value: `${program.academic_year} 學年度` },
            { label: "招生人數", value: `${program.admission_quota} 人` },
            {
                label: "考試項目",
                value: examNames.length > 0 ? examNames.join("、") : "依官方簡章為準"
            }
        ],
        eligibility: eligibility || "請參閱當年度官方簡章。",
        examFormat: examNames.length > 0 ? examNames.join("、") : "依官方簡章為準",
        fee: "依官方簡章公告為準",
        timeline:
            scheduleItems
                .filter((item) => item.id !== "pending")
                .slice(0, 3)
                .map((item) => `${item.title}：${item.date}`)
                .join("；") || "以官方簡章公告為準",
        requirements: requirementStatus(program.exam_items),
        externalLinks,
        schoolOfficialUrl: hasValue(program.school_official_url)
            ? program.school_official_url
            : undefined,
        departmentOfficialUrl: hasValue(program.department_official_url)
            ? program.department_official_url
            : undefined,
        registrationTimeline: scheduleItems,
        // Cross-year data isn't available from a single program record —
        // callers that need it fetch getAdmissionProgramHistory() separately
        // and merge it via programHistoryFromYears() (see program-view.tsx).
        history: []
    };
}

export function admissionProgramPreviews(program: AdmissionProgram): OpenGraphPreview[] {
    const brochure = admissionProgramToBrochure(program);
    return brochure.externalLinks.map((link) => {
        let siteName = "官方招生網站";
        try {
            siteName = new URL(link.url).hostname;
        } catch {
            // keep the fallback title rather than breaking the page
        }
        return {
            url: link.url,
            title: link.fallbackTitle ?? siteName,
            siteName
        };
    });
}
