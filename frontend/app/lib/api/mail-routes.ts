import { apiFetch } from "./client";

export interface MailRoute {
    id: string;
    local_part: string;
    label: string;
    discord_forum_channel_id: string;
    visible_role_ids: string[];
    // null when this route uses the global default template — see
    // MailRouteUpdateInput.reply_template.
    reply_template: string | null;
    created_at: string;
}

export interface MailRouteInput {
    local_part: string;
    label: string;
    visible_role_ids: string[];
    reply_template: string | null;
}

export interface MailRouteUpdateInput {
    label: string;
    visible_role_ids: string[];
    // null or "" clears the override back to the global default; otherwise
    // must contain both [內容] and [簽名].
    reply_template: string | null;
}

export interface DiscordRole {
    id: string;
    name: string;
}

export interface MailRouteInquiry {
    id: string;
    sender_email: string;
    note: string;
    discord_thread_id: string;
    message_count: number;
    created_at: string;
}

function mfaHeader(code?: string): Record<string, string> | undefined {
    return code ? { "X-MFA-Code": code } : undefined;
}

export function listMailRoutes(mfaCode?: string) {
    return apiFetch<{ data: MailRoute[] }>("/api/v1/admin/mail-routes", {
        headers: mfaHeader(mfaCode)
    });
}

export function createMailRoute(input: MailRouteInput, mfaCode?: string) {
    return apiFetch<{ data: MailRoute }>("/api/v1/admin/mail-routes", {
        method: "POST",
        body: input,
        headers: mfaHeader(mfaCode)
    });
}

export function updateMailRoute(id: string, input: MailRouteUpdateInput, mfaCode?: string) {
    return apiFetch<{ data: MailRoute }>(`/api/v1/admin/mail-routes/${encodeURIComponent(id)}`, {
        method: "PUT",
        body: input,
        headers: mfaHeader(mfaCode)
    });
}

export function deleteMailRoute(id: string, mfaCode?: string) {
    return apiFetch<void>(`/api/v1/admin/mail-routes/${encodeURIComponent(id)}`, {
        method: "DELETE",
        headers: mfaHeader(mfaCode)
    });
}

/** GET /api/v1/admin/mail-routes/discord-roles — 503 when Discord isn't configured. */
export function listDiscordRoles(mfaCode?: string) {
    return apiFetch<{ data: DiscordRole[] }>("/api/v1/admin/mail-routes/discord-roles", {
        headers: mfaHeader(mfaCode)
    });
}

export function listMailRouteInquiries(mailRouteId: string, mfaCode?: string) {
    return apiFetch<{ data: MailRouteInquiry[] }>(
        `/api/v1/admin/mail-routes/${encodeURIComponent(mailRouteId)}/inquiries`,
        { headers: mfaHeader(mfaCode) }
    );
}

export interface InquiryMessage {
    direction: "inbound" | "outbound";
    subject: string;
    body: string;
    actor: string;
    /** Outbound only — the replying staff member's Discord snowflake, which
     * (unlike `actor`, their display name) can't be changed after the fact. */
    actor_discord_id?: string;
    to_address?: string;
    /** Inbound only — the connecting SMTP client's address. */
    sender_ip?: string;
    /** Inbound only — Postfix/OpenDKIM's raw Authentication-Results verdict
     * (SPF/DKIM/DMARC). */
    auth_results?: string;
    /** Inbound only — the raw RFC 5322 Message-ID/In-Reply-To/References
     * threading headers. */
    source_message_id?: string;
    in_reply_to?: string;
    references?: string;
    /** Inbound only, self-reported by the sending client — informational,
     * not a security guarantee. */
    mailer?: string;
    created_at: string;
}

export interface InquiryDetail {
    sender_email: string;
    messages: InquiryMessage[];
}

export function getInquiryDetail(inquiryId: string, mfaCode?: string) {
    return apiFetch<{ data: InquiryDetail }>(`/api/v1/admin/inquiries/${encodeURIComponent(inquiryId)}`, {
        headers: mfaHeader(mfaCode)
    });
}
