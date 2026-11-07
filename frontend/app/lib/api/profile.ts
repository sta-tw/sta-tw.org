import { getCurrentAccount } from "./auth";
import { API_BASE_URL, apiFetch, apiUpload } from "./client";
import { mockResponse, mockStore } from "./mock";
import { ApiError } from "./types";

export interface ProfileLink {
    label: string;
    url: string;
}

/** profile.Profile from GET /api/v1/profile and GET /api/v1/users/{username}. */
export interface Profile {
    account_id: string;
    username: string;
    identity_status: "temporary" | "student" | "senior";
    display_name?: string;
    bio?: string;
    links: ProfileLink[];
    has_avatar: boolean;
    avatar_updated_at?: string;
    updated_at?: string;
}

interface ProfileInput {
    display_name: string;
    bio: string;
    links: ProfileLink[];
}

/** Display labels for identity_status. "senior" is a former 特選錄取生:
 * the backend's annual maintenance moves `student` accounts there once
 * their student verification expires. */
export const identityStatusLabels: Record<Profile["identity_status"], string> = {
    temporary: "未驗證",
    student: "本年度特選錄取生",
    senior: "往年特選錄取生（學長姐）"
};

/** The name to show for a profile: display_name, else the username. */
export function displayNameOf(profile: Pick<Profile, "display_name" | "username">) {
    return profile.display_name?.trim() || profile.username;
}

export const profileLimits = { displayName: 80, bio: 500, links: 10 } as const;

/** GET /api/v1/profile */
export function getMyProfile(options?: { signal?: AbortSignal }) {
    return apiFetch<{ data: Profile }>("/api/v1/profile", { signal: options?.signal });
}

/** PUT /api/v1/profile — full replace of the editable fields. */
export function updateMyProfile(input: ProfileInput) {
    return apiFetch<{ data: Profile }>("/api/v1/profile", { method: "PUT", body: input });
}

/** POST /api/v1/profile/avatar — PNG/JPEG, ≤ 2 MB. */
export function uploadAvatar(file: File) {
    const form = new FormData();
    form.append("file", file);
    return apiUpload<{ data: Profile }>("/api/v1/profile/avatar", form);
}

/** DELETE /api/v1/profile/avatar */
export function deleteAvatar() {
    return apiFetch<void>("/api/v1/profile/avatar", { method: "DELETE" });
}

export const AVATAR_MAX_BYTES = 2 * 1024 * 1024;
export const AVATAR_ACCEPT = "image/png,image/jpeg";

/** <img> src for a profile's avatar (the endpoint 302s to a presigned
 * URL), versioned by avatar_updated_at so a new upload busts the cache;
 * null when there is no avatar. */
export function profileAvatarSrc(
    profile: Pick<Profile, "username" | "has_avatar" | "avatar_updated_at"> | null
) {
    if (!profile?.has_avatar) return null;
    const version = profile.avatar_updated_at;
    const query = version ? `?v=${encodeURIComponent(version)}` : "";
    return `${API_BASE_URL}/api/v1/users/${encodeURIComponent(profile.username)}/avatar${query}`;
}

/** GET /api/v1/users/{username}. Currently requires a session (401 when
 * signed out); opening it to visitors is pending backend work. */
export function getPublicProfile(username: string, options?: { signal?: AbortSignal }) {
    return apiFetch<{ data: Profile }>(`/api/v1/users/${encodeURIComponent(username)}`, {
        signal: options?.signal
    });
}

// ---------------------------------------------------------------------------
// Not backed yet — see lib/api/mock.ts.

export interface ProfileDetails {
    school_name: string;
    department: string;
    hide_school: boolean;
    hide_department: boolean;
    /** Custom URL segment for /user/{slug}; can be set once, then locked. */
    profile_slug: string | null;
}

export interface PrivacySettings {
    /** "members" hides the public profile from signed-out visitors. */
    profile_visibility: "public" | "members";
    show_articles: boolean;
    show_portfolio: boolean;
}

/** What GET /api/v1/users/{username}/details returns: hidden fields omitted. */
export interface PublicProfileDetails {
    school_name?: string;
    department?: string;
    show_articles: boolean;
    show_portfolio: boolean;
}

const emptyDetails: ProfileDetails = {
    school_name: "",
    department: "",
    hide_school: false,
    hide_department: false,
    profile_slug: null
};

const defaultPrivacy: PrivacySettings = {
    profile_visibility: "public",
    show_articles: true,
    show_portfolio: true
};

// The mock only knows this browser's own account, stored with its username
// so the public-profile lookup can tell "my profile" from anyone else's.
type StoredDetails = ProfileDetails & { username?: string };
const DETAILS_KEY = "profile-details";
const PRIVACY_KEY = "profile-privacy";

/** MOCK: GET /api/v1/profile/details */
export function getMyProfileDetails(options?: { signal?: AbortSignal }) {
    return mockResponse(() => {
        const stored = mockStore.read<StoredDetails>(DETAILS_KEY, emptyDetails);
        const details: ProfileDetails = {
            school_name: stored.school_name,
            department: stored.department,
            hide_school: stored.hide_school,
            hide_department: stored.hide_department,
            profile_slug: stored.profile_slug
        };
        return { data: details };
    }, options?.signal);
}

/** MOCK: PUT /api/v1/profile/details — 409 `slug_locked` if the slug changes
 * after it was set. */
export function updateMyProfileDetails(details: ProfileDetails) {
    return mockResponse(async () => {
        const current = mockStore.read<StoredDetails>(DETAILS_KEY, emptyDetails);
        if (current.profile_slug && details.profile_slug !== current.profile_slug) {
            throw new ApiError(409, "slug_locked", "自訂網址設定後就無法更改。");
        }
        const { account } = await getCurrentAccount();
        mockStore.write<StoredDetails>(DETAILS_KEY, { ...details, username: account.username });
        return { data: details };
    });
}

/** MOCK: GET /api/v1/profile/privacy */
export function getPrivacySettings(options?: { signal?: AbortSignal }) {
    return mockResponse(
        () => ({ data: mockStore.read(PRIVACY_KEY, defaultPrivacy) }),
        options?.signal
    );
}

/** MOCK: PUT /api/v1/profile/privacy */
export function updatePrivacySettings(settings: PrivacySettings) {
    return mockResponse(() => {
        mockStore.write(PRIVACY_KEY, settings);
        return { data: settings };
    });
}

/** MOCK: GET /api/v1/users/resolve/{segment} — the username behind a
 * /user/{segment} URL, which may be a custom slug. The mock only knows this
 * browser's own slug; any other segment is taken as a username. */
export function resolveUser(segment: string, options?: { signal?: AbortSignal }) {
    return mockResponse(() => {
        const details = mockStore.read<StoredDetails>(DETAILS_KEY, emptyDetails);
        const isMySlug = details.profile_slug === segment && details.username;
        return { data: { username: isMySlug ? details.username! : segment } };
    }, options?.signal);
}

/** MOCK: GET /api/v1/users/{username}/details — returns the stored details
 * only for this browser's own account, defaults for anyone else. */
export function getPublicProfileDetails(username: string, options?: { signal?: AbortSignal }) {
    return mockResponse((): { data: PublicProfileDetails } => {
        const details = mockStore.read<StoredDetails>(DETAILS_KEY, emptyDetails);
        const privacy = mockStore.read(PRIVACY_KEY, defaultPrivacy);
        if (username !== details.username)
            return { data: { show_articles: true, show_portfolio: true } };
        return {
            data: {
                school_name: details.hide_school ? undefined : details.school_name || undefined,
                department: details.hide_department ? undefined : details.department || undefined,
                show_articles: privacy.show_articles,
                show_portfolio: privacy.show_portfolio
            }
        };
    }, options?.signal);
}

/** A published portfolio file as it will appear on someone's public profile. */
export interface PublicPortfolioItem {
    file_id: string;
    project_title: string;
    school_name: string;
    program_name: string;
    academic_year: number;
    published_at: string;
}

/** MOCK: GET /api/v1/users/{username}/portfolio — published files only. There
 * is no public portfolio listing yet, so the mock is always empty. */
export function listUserPortfolio(username: string, options?: { signal?: AbortSignal }) {
    void username;
    return mockResponse((): { data: PublicPortfolioItem[] } => ({ data: [] }), options?.signal);
}
