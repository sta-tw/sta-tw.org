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

export type NotificationGroup = "personal" | "interaction";

export interface NotificationKind {
    kind: string;
    group: NotificationGroup;
    label: string;
    /** On for a new account and after 恢復預設值. */
    defaultEnabled: boolean;
}

/** Every kind users can toggle. `willingness` and `support` are what the
 * backend emits today (notifications/worker.go, support/postgres_repository.go);
 * the rest are the kinds the design asks for and the backend will add. */
export const NOTIFICATION_KINDS: readonly NotificationKind[] = [
    {
        kind: "verification_open",
        group: "personal",
        label: "下一屆特選生身分開放申請",
        defaultEnabled: false
    },
    {
        kind: "saved_program_schedule",
        group: "personal",
        label: "已儲存的簡章校系開放／截止報名",
        defaultEnabled: false
    },
    {
        kind: "willingness",
        group: "personal",
        label: "交叉查榜填寫、錄取通知",
        defaultEnabled: true
    },
    { kind: "support", group: "personal", label: "客服回覆", defaultEnabled: true },
    {
        kind: "portfolio_views",
        group: "interaction",
        label: "你的交換／公開備審達到 100、500 及 1000 次瀏覽",
        defaultEnabled: false
    },
    {
        kind: "portfolio_likes",
        group: "interaction",
        label: "有人按讚你的交換／公開備審",
        defaultEnabled: false
    },
    {
        kind: "experience_views",
        group: "interaction",
        label: "你的文章達到 100、500 及 1000 次瀏覽",
        defaultEnabled: false
    },
    {
        kind: "experience_reactions",
        group: "interaction",
        label: "有人按讚你的文章",
        defaultEnabled: false
    },
    {
        kind: "forum_replies",
        group: "interaction",
        label: "有人在你的論壇貼文留言",
        defaultEnabled: true
    },
    {
        kind: "forum_mentions",
        group: "interaction",
        label: "有人在論壇提及你",
        defaultEnabled: true
    }
];

export const notificationKindLabel = (kind: string) =>
    NOTIFICATION_KINDS.find((item) => item.kind === kind)?.label ?? "系統通知";

// ---------------------------------------------------------------------------
// Not backed yet — see lib/api/mock.ts.

export type NotificationChannel = "email" | "discord";

export interface NotificationPreferences {
    /** Where notifications are sent besides the site itself. Discord needs
     * a linked Discord account (DMs from the 特選群 bot). */
    channels: Record<NotificationChannel, boolean>;
    /** Per-kind on/off; a kind missing from the map uses its default. */
    kinds: Record<string, boolean>;
}

export const DEFAULT_NOTIFICATION_PREFERENCES: NotificationPreferences = {
    channels: { email: true, discord: false },
    kinds: Object.fromEntries(NOTIFICATION_KINDS.map((item) => [item.kind, item.defaultEnabled]))
};

const PREFERENCES_KEY = "notification-preferences";

/** MOCK: GET /api/v1/notifications/preferences */
export function getNotificationPreferences(options?: { signal?: AbortSignal }) {
    return mockResponse(() => {
        // Older mock data stored a single `email_enabled` flag.
        const stored = mockStore.read<
            Partial<NotificationPreferences> & { email_enabled?: boolean }
        >(PREFERENCES_KEY, {});
        const data: NotificationPreferences = {
            channels: {
                ...DEFAULT_NOTIFICATION_PREFERENCES.channels,
                ...(stored.email_enabled === undefined ? {} : { email: stored.email_enabled }),
                ...stored.channels
            },
            kinds: { ...DEFAULT_NOTIFICATION_PREFERENCES.kinds, ...stored.kinds }
        };
        return { data };
    }, options?.signal);
}

/** MOCK: PUT /api/v1/notifications/preferences */
export function updateNotificationPreferences(input: NotificationPreferences) {
    return mockResponse(() => {
        mockStore.write(PREFERENCES_KEY, input);
        return { data: input };
    });
}
