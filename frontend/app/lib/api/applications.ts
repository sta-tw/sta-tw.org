import { apiFetch } from "./client";
import { mockResponse, mockStore } from "./mock";
import { ApiError } from "./types";

/** applications.Application — a 校系 the student applied to. Unrelated to
 * account-applications.ts (the sign-up request flow). */
export interface Application {
    id: string;
    program_identifier: string;
    school_code: string;
    school_name: string;
    program_code: string;
    program_name: string;
    academic_year: number;
    status: string;
    locked_at?: string;
    /** Last four characters of the saved 准考證號碼; MOCK field, see below. */
    candidate_number_last4?: string;
    /** 1-based 志願序 among applications with a candidate number; MOCK. */
    preference_rank?: number;
}

/** "115 學年度 國立臺灣大學 資訊工程學系" */
export const applicationLabel = (application: Application) =>
    `${application.academic_year} 學年度 ${application.school_name} ${application.program_name}`;

/** POST /api/v1/applications accepts 1..50 identifiers per request. */
export const APPLICATION_BATCH_LIMIT = 50;

/** GET /api/v1/applications — needs a verified identity (403
 * verification_required otherwise). Wrapped below. */
function realListApplications(options?: { signal?: AbortSignal }) {
    return apiFetch<{ data: Application[] }>("/api/v1/applications", {
        signal: options?.signal
    });
}

/** POST /api/v1/applications — creates confirmed (locked) applications;
 * student identity only. */
export function createApplications(programIdentifiers: string[]) {
    return apiFetch<{ data: Application[] }>("/api/v1/applications", {
        method: "POST",
        body: { program_identifiers: programIdentifiers }
    });
}

export type ResultStatus = "admitted" | "waitlisted" | "rejected" | "unknown";

/** results.Report — 404 result_not_found until the official list is out. */
export interface ApplicationResult {
    application_id: string;
    result_status: ResultStatus;
    official_rank?: number;
    quota?: number;
    front_candidate_count: number;
    front_response_count: number;
    position_after_declines?: number;
    /** Already a percentage (0–100), averaged from willingness values. */
    reference_probability?: number;
    current_willingness?: number;
    candidate_number_last4?: string;
    candidate_number_set: boolean;
}

export interface ApplicationInquiry {
    id: string;
    round: "result_released" | "acceptance_deadline";
    response_deadline?: string;
    created_at: string;
    responded: boolean;
    current_willingness?: number;
}

/** results.validWillingness */
export const WILLINGNESS_VALUES = [0, 20, 40, 60, 80, 100] as const;

/** GET /api/v1/applications/{applicationID}/result */
export function getApplicationResult(applicationID: string, options?: { signal?: AbortSignal }) {
    return apiFetch<{ data: ApplicationResult }>(
        `/api/v1/applications/${encodeURIComponent(applicationID)}/result`,
        { signal: options?.signal }
    );
}

/** GET /api/v1/applications/{applicationID}/inquiries */
export function listApplicationInquiries(
    applicationID: string,
    options?: { signal?: AbortSignal }
) {
    return apiFetch<{ data: ApplicationInquiry[] }>(
        `/api/v1/applications/${encodeURIComponent(applicationID)}/inquiries`,
        { signal: options?.signal }
    );
}

/** PUT /api/v1/applications/{applicationID}/candidate-number — 4–64 chars
 * of [A-Za-z0-9-_]; responds 204. Wrapped below. */
function realSetCandidateNumber(applicationID: string, candidateNumber: string) {
    return apiFetch<void>(
        `/api/v1/applications/${encodeURIComponent(applicationID)}/candidate-number`,
        { method: "PUT", body: { candidate_number: candidateNumber } }
    );
}

/** PUT /api/v1/applications/{applicationID}/willingness */
export function setWillingness(
    applicationID: string,
    input: { value: (typeof WILLINGNESS_VALUES)[number]; inquiry_id?: string }
) {
    return apiFetch<{ data: unknown }>(
        `/api/v1/applications/${encodeURIComponent(applicationID)}/willingness`,
        { method: "PUT", body: input }
    );
}

/** 報到意願 wording for each willingness value, strongest first. */
export const willingnessLabels: Record<(typeof WILLINGNESS_VALUES)[number], string> = {
    100: "必報到",
    80: "很可能報到",
    60: "傾向報到",
    40: "觀望中",
    20: "不太可能報到",
    0: "確定放棄"
};

// ---------------------------------------------------------------------------
// Not backed yet — see lib/api/mock.ts. The backend only reveals whether a
// candidate number is set through the result (404 until results are out) and
// has no 志願序, so this browser remembers both; the anonymous per-rank board
// for 交叉查榜查詢 doesn't exist yet either.

const CANDIDATE_KEY = "application-candidate-numbers";
const ORDER_KEY = "application-preference-order";
const BOARD_KEY = "application-result-boards";

/** GET /api/v1/applications with MOCK: `candidate_number_last4` and
 * `preference_rank` on every item. */
export async function listApplications(options?: {
    signal?: AbortSignal;
}): Promise<{ data: Application[] }> {
    const { data } = await realListApplications(options);
    const last4 = mockStore.read<Record<string, string>>(CANDIDATE_KEY, {});
    const order = mockStore
        .read<string[]>(ORDER_KEY, [])
        .filter((id) => last4[id] && data.some((item) => item.id === id));
    // Applications given a number but missing from the stored order go last.
    for (const item of data) if (last4[item.id] && !order.includes(item.id)) order.push(item.id);
    return {
        data: data.map((item) => ({
            ...item,
            candidate_number_last4: last4[item.id],
            preference_rank: order.includes(item.id) ? order.indexOf(item.id) + 1 : undefined
        }))
    };
}

/** PUT /api/v1/applications/{applicationID}/candidate-number; MOCK: the
 * last four characters are remembered for listApplications. */
export async function setCandidateNumber(applicationID: string, candidateNumber: string) {
    await realSetCandidateNumber(applicationID, candidateNumber);
    mockStore.write(CANDIDATE_KEY, {
        ...mockStore.read<Record<string, string>>(CANDIDATE_KEY, {}),
        [applicationID]: candidateNumber.slice(-4)
    });
}

/** MOCK: PUT /api/v1/applications/preferences — the full 志願序, most
 * wanted first; only applications with a candidate number can be ranked. */
export function setApplicationPreferences(applicationIDs: string[]) {
    return mockResponse(() => {
        mockStore.write(ORDER_KEY, applicationIDs);
    });
}

/** One row of the anonymous 交叉查榜 board: everyone on the official list
 * for the program, in list order. */
export interface ResultBoardRow {
    /** "正取" or "備取 3" style label from the official list. */
    label: string;
    result_status: "admitted" | "waitlisted";
    /** Whether that candidate has registered for 交叉查榜 here. */
    registered: boolean;
    willingness?: (typeof WILLINGNESS_VALUES)[number];
    is_me: boolean;
}

/** MOCK: GET /api/v1/applications/{applicationID}/result/board — 404
 * result_not_found until the list is out, like the result itself. */
export function getResultBoard(applicationID: string, options?: { signal?: AbortSignal }) {
    return mockResponse(() => {
        const board = mockStore.read<Record<string, ResultBoardRow[]>>(BOARD_KEY, {})[
            applicationID
        ];
        if (!board) throw new ApiError(404, "result_not_found", "榜單尚未公布。");
        return { data: board };
    }, options?.signal);
}
