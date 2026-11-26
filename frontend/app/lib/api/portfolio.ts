import { getCurrentAccount } from "./auth";
import { mockResponse, mockStore } from "./mock";
import { displayNameOf, getMyProfile } from "./profile";
import { ApiError } from "./types";

/** Who can see a portfolio: only its author, verified 特選生 (交換備審 —
 * shared with those who share theirs), or everyone (公開備審). */
export type PortfolioVisibility = "private" | "exchange" | "public";

/** Review state of the latest saved version. Private portfolios never go
 * through review and stay "draft". */
export type PortfolioReviewStatus = "draft" | "pending_review" | "approved" | "rejected";

/** One named document of a portfolio (自傳, 讀書計畫, ...), always a PDF. */
export interface PortfolioItem {
    id: string;
    name: string;
    file_name: string;
    file_size_bytes: number;
    updated_at: string;
}

/** A 備審 for one 校系: several named PDFs shared at one visibility. */
export interface Portfolio {
    id: string;
    academic_year: number;
    school_name: string;
    program_name: string;
    visibility: PortfolioVisibility;
    review_status: PortfolioReviewStatus;
    rejection_reason?: string;
    items: PortfolioItem[];
    author_username: string;
    author_name: string;
    like_count: number;
    /** Whether the caller liked / saved it (false when signed out). */
    liked: boolean;
    saved: boolean;
    created_at: string;
    updated_at: string;
}

export const portfolioVisibilityLabels: Record<PortfolioVisibility, string> = {
    private: "私人備審",
    exchange: "交換備審",
    public: "公開備審"
};

/** "國立清華大學 藝術學院學士班" */
export const portfolioLabel = (portfolio: Pick<Portfolio, "school_name" | "program_name">) =>
    `${portfolio.school_name} ${portfolio.program_name}`;

/** Newest update across the portfolio and its items. */
export const portfolioUpdatedAt = (portfolio: Portfolio) =>
    portfolio.items.reduce(
        (latest, item) => (item.updated_at > latest ? item.updated_at : latest),
        portfolio.updated_at
    );

export const PORTFOLIO_ITEM_NAME_MAX_LENGTH = 25;
export const PORTFOLIO_ITEM_LIMIT = 10;
export const PORTFOLIO_FILE_MAX_BYTES = 20 * 1024 * 1024;
export const PORTFOLIO_FILE_ACCEPT = ".pdf,application/pdf";

export interface PortfolioItemInput {
    /** Set when keeping or renaming an existing item. */
    id?: string;
    name: string;
    /** Required for a new item; replaces the file of an existing one. */
    file?: File;
}

export interface PortfolioInput {
    academic_year: number;
    school_name: string;
    program_name: string;
    visibility: PortfolioVisibility;
    items: PortfolioItemInput[];
}

// ---------------------------------------------------------------------------
// Not backed yet — see lib/api/mock.ts. The backend's portfolio is one
// versioned file per application (/portfolio/projects); the design needs
// several named files per 校系 with a visibility, plus browsing, likes and
// bookmarks, so the whole surface is mocked here.
//
// Every portfolio this browser created lives under one key with its owner,
// so the browse/profile mocks can list other people's too (e.g. entries
// pre-filled for testing). Review never happens in the mock: a submitted
// portfolio stays pending_review, and only "approved" ones are listed.

const PORTFOLIOS_KEY = "portfolios";
const LIKES_KEY = "portfolio-likes";
const SAVED_KEY = "saved-portfolios";

type StoredPortfolio = Omit<Portfolio, "liked" | "saved">;

const readPortfolios = () => mockStore.read<StoredPortfolio[]>(PORTFOLIOS_KEY, []);
const readLikes = () => mockStore.read<string[]>(LIKES_KEY, []);
const readSaved = () => mockStore.read<{ portfolio_id: string; saved_at: string }[]>(SAVED_KEY, []);

/** Uploaded files are kept in memory only, so downloads work for files
 * picked in this tab; the mock has nowhere to persist PDFs. */
const uploadedFiles = new Map<string, File>();

function withViewerState(portfolio: StoredPortfolio): Portfolio {
    return {
        ...portfolio,
        liked: readLikes().includes(portfolio.id),
        saved: readSaved().some((item) => item.portfolio_id === portfolio.id)
    };
}

async function currentUsername() {
    const me = await getCurrentAccount().catch(() => null);
    return me?.account.username ?? null;
}

function notFound(): never {
    throw new ApiError(404, "not_found", "找不到這份備審。");
}

function requireSignIn(username: string | null): asserts username is string {
    if (!username) throw new ApiError(401, "unauthorized", "請先登入。");
}

/** The signed-in account, or a 401 like the real endpoint. */
async function requireAccount() {
    const me = await getCurrentAccount().catch(() => null);
    if (!me) throw new ApiError(401, "unauthorized", "請先登入。");
    return me;
}

const newId = () => crypto.randomUUID();

/** MOCK: GET /api/v1/me/portfolios — the caller's portfolios in every
 * visibility, most recently changed first. */
export function listMyPortfolios(options?: { signal?: AbortSignal }) {
    return mockResponse(async () => {
        const username = await currentUsername();
        requireSignIn(username);
        return {
            data: readPortfolios()
                .filter((item) => item.author_username === username)
                .map(withViewerState)
        };
    }, options?.signal);
}

/** MOCK: GET /api/v1/me/portfolios/{portfolioID} */
export function getMyPortfolio(portfolioID: string, options?: { signal?: AbortSignal }) {
    return mockResponse(async () => {
        const username = await currentUsername();
        const portfolio = readPortfolios().find(
            (item) => item.id === portfolioID && item.author_username === username
        );
        if (!portfolio) notFound();
        return { data: withViewerState(portfolio) };
    }, options?.signal);
}

/** MOCK: POST /api/v1/portfolios (new) or PUT /api/v1/portfolios/{id}
 * (multipart: the JSON fields plus one `file:<index>` part per new or
 * replaced item). Saving resets the review to draft; exchange needs a
 * verified identity (403 verification_required). */
export function savePortfolio(input: PortfolioInput, portfolioID?: string) {
    return mockResponse(async () => {
        const me = await requireAccount();
        if (input.visibility === "exchange" && me.account.identity_status === "temporary") {
            throw new ApiError(403, "verification_required", "交換備審需先完成身分驗證。");
        }
        const all = readPortfolios();
        const existing = portfolioID
            ? all.find(
                  (item) => item.id === portfolioID && item.author_username === me.account.username
              )
            : undefined;
        if (portfolioID && !existing) notFound();

        const now = new Date().toISOString();
        const items = input.items.map((item): PortfolioItem => {
            const previous = existing?.items.find((old) => old.id === item.id);
            const id = previous?.id ?? newId();
            if (item.file) uploadedFiles.set(id, item.file);
            if (!previous && !item.file) {
                throw new ApiError(422, "validation_failed", `「${item.name}」還沒有上傳檔案。`);
            }
            return {
                id,
                name: item.name.trim(),
                file_name: item.file?.name ?? previous!.file_name,
                file_size_bytes: item.file?.size ?? previous!.file_size_bytes,
                updated_at:
                    item.file || item.name.trim() !== previous?.name ? now : previous.updated_at
            };
        });

        const profile = await getMyProfile()
            .then(({ data }) => data)
            .catch(() => null);
        const saved: StoredPortfolio = {
            id: existing?.id ?? newId(),
            academic_year: input.academic_year,
            school_name: input.school_name.trim(),
            program_name: input.program_name.trim(),
            visibility: input.visibility,
            review_status: "draft",
            items,
            author_username: me.account.username,
            author_name: profile ? displayNameOf(profile) : me.account.username,
            like_count: existing?.like_count ?? 0,
            created_at: existing?.created_at ?? now,
            updated_at: now
        };
        mockStore.write(PORTFOLIOS_KEY, [saved, ...all.filter((item) => item.id !== saved.id)]);
        return { data: withViewerState(saved) };
    });
}

function updateMine(portfolioID: string, update: (portfolio: StoredPortfolio) => StoredPortfolio) {
    return mockResponse(async () => {
        const username = await currentUsername();
        const all = readPortfolios();
        const target = all.find(
            (item) => item.id === portfolioID && item.author_username === username
        );
        if (!target) notFound();
        const next = update(target);
        mockStore.write(
            PORTFOLIOS_KEY,
            all.map((item) => (item.id === portfolioID ? next : item))
        );
        return { data: withViewerState(next) };
    });
}

/** MOCK: POST /api/v1/portfolios/{portfolioID}/submit — sends an exchange
 * or public portfolio to admin review (draft / rejected → pending_review). */
export function submitPortfolio(portfolioID: string) {
    return updateMine(portfolioID, (portfolio) => {
        if (portfolio.visibility === "private") {
            throw new ApiError(409, "invalid_state", "私人備審不需要送審。");
        }
        return { ...portfolio, review_status: "pending_review", rejection_reason: undefined };
    });
}

/** MOCK: DELETE /api/v1/portfolios/{portfolioID} */
export function deletePortfolio(portfolioID: string) {
    return mockResponse(async () => {
        const username = await currentUsername();
        mockStore.write(
            PORTFOLIOS_KEY,
            readPortfolios().filter(
                (item) => !(item.id === portfolioID && item.author_username === username)
            )
        );
    });
}

/** Matches "資工" against "資訊工程學系" loosely enough for a search box:
 * every whitespace-separated term must appear in the school or program. */
function matchesQuery(portfolio: StoredPortfolio, q?: string) {
    const terms = q?.trim().toLowerCase().split(/\s+/).filter(Boolean) ?? [];
    const haystack = `${portfolio.school_name} ${portfolio.program_name} ${portfolio.author_name}`
        .toLowerCase()
        .replaceAll("臺", "台");
    return terms.every((term) => haystack.includes(term.replaceAll("臺", "台")));
}

/** MOCK: GET /api/v1/portfolios?visibility=&q= — approved portfolios,
 * newest first. Exchange needs a verified identity (403
 * verification_required); signed-out viewers get 401. */
export function browsePortfolios(options: {
    visibility: "exchange" | "public";
    q?: string;
    signal?: AbortSignal;
}) {
    return mockResponse(async () => {
        if (options.visibility === "exchange") {
            const me = await requireAccount();
            if (me.account.identity_status === "temporary") {
                throw new ApiError(403, "verification_required", "交換備審需先完成身分驗證。");
            }
        }
        const data = readPortfolios()
            .filter(
                (item) =>
                    item.visibility === options.visibility &&
                    item.review_status === "approved" &&
                    matchesQuery(item, options.q)
            )
            .sort((a, b) => b.updated_at.localeCompare(a.updated_at))
            .map(withViewerState);
        return { data };
    }, options.signal);
}

/** MOCK: GET /api/v1/users/{username}/portfolios — approved public ones. */
export function listUserPortfolios(username: string, options?: { signal?: AbortSignal }) {
    return mockResponse(
        () => ({
            data: readPortfolios()
                .filter(
                    (item) =>
                        item.author_username === username &&
                        item.visibility === "public" &&
                        item.review_status === "approved"
                )
                .map(withViewerState)
        }),
        options?.signal
    );
}

async function requireViewer() {
    requireSignIn(await currentUsername());
}

function setLike(portfolioID: string, liked: boolean) {
    return mockResponse(async () => {
        await requireViewer();
        const likes = readLikes();
        if (likes.includes(portfolioID) === liked) return;
        mockStore.write(
            LIKES_KEY,
            liked ? [...likes, portfolioID] : likes.filter((id) => id !== portfolioID)
        );
        mockStore.write(
            PORTFOLIOS_KEY,
            readPortfolios().map((item) =>
                item.id === portfolioID
                    ? { ...item, like_count: Math.max(0, item.like_count + (liked ? 1 : -1)) }
                    : item
            )
        );
    });
}

/** MOCK: PUT /api/v1/portfolios/{portfolioID}/like — idempotent. */
export const likePortfolio = (portfolioID: string) => setLike(portfolioID, true);

/** MOCK: DELETE /api/v1/portfolios/{portfolioID}/like — idempotent. */
export const unlikePortfolio = (portfolioID: string) => setLike(portfolioID, false);

function setSaved(portfolioID: string, saved: boolean) {
    return mockResponse(async () => {
        await requireViewer();
        const others = readSaved().filter((item) => item.portfolio_id !== portfolioID);
        mockStore.write(
            SAVED_KEY,
            saved
                ? [{ portfolio_id: portfolioID, saved_at: new Date().toISOString() }, ...others]
                : others
        );
    });
}

/** MOCK: PUT /api/v1/saved-portfolios/{portfolioID} — idempotent. */
export const savePortfolioBookmark = (portfolioID: string) => setSaved(portfolioID, true);

/** MOCK: DELETE /api/v1/saved-portfolios/{portfolioID} — idempotent. */
export const removePortfolioBookmark = (portfolioID: string) => setSaved(portfolioID, false);

/** MOCK: GET /api/v1/saved-portfolios — newest first; portfolios that are
 * no longer visible drop out. */
export function listSavedPortfolios(options?: { signal?: AbortSignal }) {
    return mockResponse(async () => {
        await requireViewer();
        const byId = new Map(readPortfolios().map((item) => [item.id, item]));
        const data = readSaved()
            .map((saved) => byId.get(saved.portfolio_id))
            .filter(
                (item): item is StoredPortfolio =>
                    item !== undefined &&
                    item.visibility !== "private" &&
                    item.review_status === "approved"
            )
            .map(withViewerState);
        return { data };
    }, options?.signal);
}

/** MOCK: GET /api/v1/portfolio-items/{itemID}/download — a short-lived URL
 * (not wrapped in `data`, like the existing file downloads). */
export function getPortfolioItemDownload(itemID: string) {
    return mockResponse(() => {
        const file = uploadedFiles.get(itemID);
        if (!file) {
            throw new ApiError(
                404,
                "file_unavailable",
                "測試資料只保留這個分頁上傳的檔案，目前無法下載。"
            );
        }
        return { url: URL.createObjectURL(file), expires_in: 300 };
    });
}
