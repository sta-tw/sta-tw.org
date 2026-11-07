import { apiFetch } from "./client";
import { mockResponse, mockStore } from "./mock";

export interface Notification {
    id: string;
    kind: string;
    title: string;
    body: string;
    read_at?: string;
    created_at: string;
}

/** GET /api/v1/notifications — newest first, keyset paginated (limit 1–100,
 * default 50). An empty next_cursor means there are no more pages. */
export function listNotifications(
    options: { limit?: number; cursor?: string; signal?: AbortSignal } = {}
) {
    return apiFetch<{ data: Notification[]; next_cursor: string }>("/api/v1/notifications", {
        query: { limit: options.limit, cursor: options.cursor || undefined },
        signal: options.signal
    });
}

/** POST /api/v1/notifications/{notificationID}/read — 204. */
export function markNotificationRead(notificationID: string) {
    return apiFetch<void>(`/api/v1/notifications/${encodeURIComponent(notificationID)}/read`, {
        method: "POST"
    });
}

/** POST /api/v1/notifications/read-all */
export function markAllNotificationsRead() {
    return apiFetch<{ marked_read: number }>("/api/v1/notifications/read-all", {
        method: "POST"
    });
}

/** Notification kinds the backend emits today (notifications/worker.go,
 * support/postgres_repository.go) with their user-facing labels. */
export const NOTIFICATION_KINDS = [
    {
        kind: "willingness",
        label: "查榜意願提醒",
        description: "榜單公布或正取截止前，提醒你填寫交叉查榜意願。"
    },
    {
        kind: "support",
        label: "客服回覆",
        description: "你提交的客服單有新的回覆或狀態更新。"
    }
] as const;

// ---------------------------------------------------------------------------
// Not backed yet — see lib/api/mock.ts.

export interface NotificationPreferences {
    email_enabled: boolean;
    /** Per-kind on/off; a kind missing from the map is on. */
    kinds: Record<string, boolean>;
}

const PREFERENCES_KEY = "notification-preferences";

/** MOCK: GET /api/v1/notifications/preferences */
export function getNotificationPreferences(options?: { signal?: AbortSignal }) {
    return mockResponse(
        () => ({
            data: mockStore.read<NotificationPreferences>(PREFERENCES_KEY, {
                email_enabled: true,
                kinds: {}
            })
        }),
        options?.signal
    );
}

/** MOCK: PUT /api/v1/notifications/preferences */
export function updateNotificationPreferences(input: NotificationPreferences) {
    return mockResponse(() => {
        mockStore.write(PREFERENCES_KEY, input);
        return { data: input };
    });
}
