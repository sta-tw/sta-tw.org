import { apiFetch } from "./client";

export interface CalendarStatus {
    linked: boolean;
    /** external_ids (see CalendarEventInput.external_id) this account
     * currently has a calendar event recorded for — only present when
     * linked is true. Lets the caller offer "remove" instead of "add" for
     * items already on the calendar. */
    linked_event_ids?: string[];
}

/** GET /api/v1/calendar/google/status — rejects with ApiError(401) when not logged in. */
export function getCalendarStatus(options?: { signal?: AbortSignal }) {
    return apiFetch<CalendarStatus>("/api/v1/calendar/google/status", { signal: options?.signal });
}

export interface CalendarEventInput {
    title: string;
    /** "YYYY-MM-DD", inclusive. */
    iso_start: string;
    /** "YYYY-MM-DD", inclusive (equal to iso_start for a single-day event). */
    iso_end: string;
    details: string;
    /** Caller-chosen stable id for this event (e.g. `${programIdentifier}:${sortOrder}`).
     * When set, a successful insert is recorded under it so it can later be
     * removed via removeCalendarEvent(); omit for a fire-and-forget insert. */
    external_id?: string;
}

export interface CreateCalendarEventsResult {
    created: number;
}

/** POST /api/v1/calendar/google/events — rejects with ApiError(428, "calendar_not_linked")
 * when the account hasn't granted calendar access yet; the caller should send the
 * visitor through startCalendarLink() and retry after they return. */
export function createCalendarEvents(items: CalendarEventInput[]) {
    return apiFetch<CreateCalendarEventsResult>("/api/v1/calendar/google/events", {
        method: "POST",
        body: { items }
    });
}

/** DELETE /api/v1/calendar/google/events/{externalId} — removes a
 * previously-added event from the user's Google Calendar. Idempotent: also
 * resolves (no error) if the event was already removed. Rejects with
 * ApiError(404) if this externalId was never added in the first place. */
export function removeCalendarEvent(externalId: string) {
    return apiFetch<void>(`/api/v1/calendar/google/events/${encodeURIComponent(externalId)}`, {
        method: "DELETE"
    });
}

/** Requests the Google consent URL for calendar access (bound to the caller's
 * already-logged-in STA account); the caller navigates the browser there
 * (`window.location.href = result.authorization_url`). `returnTo` must be a
 * same-origin relative path — the backend lands the browser back there,
 * appending `?oauth=success`, once the OAuth round trip completes. */
export function startCalendarLink(returnTo: string) {
    return apiFetch<{ authorization_url: string }>("/api/v1/auth/oauth/google/bind/start", {
        method: "POST",
        query: { return_to: returnTo }
    });
}
