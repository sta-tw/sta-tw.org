import { API_BASE_URL, apiFetch, apiUpload } from "./client";

export interface ProfileLink {
    label: string;
    url: string;
}

/** profile.Profile from GET /api/v1/profile and GET /api/v1/users/{username}. */
export interface Profile {
    account_id: string;
    username: string;
    identity_status: "temporary" | "student" | "senior" | "parent" | "educator";
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
    senior: "往年特選錄取生（學長姐）",
    parent: "特選生家長",
    educator: "師長／輔導老師"
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

export interface ProfileDetails {
    school_name: string;
    school_selection_type: "none" | "directory" | "other_status" | "unlisted";
    school_directory_id: string | null;
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

/** GET /api/v1/profile/details */
export function getMyProfileDetails(options?: { signal?: AbortSignal }) {
    return apiFetch<{ data: ProfileDetails }>("/api/v1/profile/details", {
        signal: options?.signal
    });
}

/** PUT /api/v1/profile/details — the slug is immutable once set. */
export function updateMyProfileDetails(details: ProfileDetails) {
    return apiFetch<{ data: ProfileDetails }>("/api/v1/profile/details", {
        method: "PUT",
        body: details
    });
}

/** GET /api/v1/profile/privacy */
export function getPrivacySettings(options?: { signal?: AbortSignal }) {
    return apiFetch<{ data: PrivacySettings }>("/api/v1/profile/privacy", {
        signal: options?.signal
    });
}

/** PUT /api/v1/profile/privacy */
export function updatePrivacySettings(settings: PrivacySettings) {
    return apiFetch<{ data: PrivacySettings }>("/api/v1/profile/privacy", {
        method: "PUT",
        body: settings
    });
}

/** GET /api/v1/users/resolve?segment=... — resolves username or custom slug. */
export function resolveUser(segment: string, options?: { signal?: AbortSignal }) {
    return apiFetch<{ data: { username: string } }>("/api/v1/users/resolve", {
        query: { segment },
        signal: options?.signal
    });
}

/** GET /api/v1/users/{username}/details — privacy-filtered profile details. */
export function getPublicProfileDetails(username: string, options?: { signal?: AbortSignal }) {
    return apiFetch<{ data: PublicProfileDetails }>(
        `/api/v1/users/${encodeURIComponent(username)}/details`,
        { signal: options?.signal }
    );
}
