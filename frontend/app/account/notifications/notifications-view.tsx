"use client";

import { Bell, CheckCheck } from "lucide-react";
import { useState } from "react";
import { twMerge } from "tailwind-merge";
import Button from "../../components/button";
import { Card } from "../../components/card";
import Checkbox from "../../components/checkbox";
import {
    getNotificationPreferences,
    listNotifications,
    markAllNotificationsRead,
    markNotificationRead,
    NOTIFICATION_KINDS,
    notificationKindLabel,
    updateNotificationPreferences,
    type Notification,
    type NotificationPreferences
} from "../../lib/api/notifications";
import AccountPage from "../account-page";
import { FieldGroup } from "../../components/field";
import { EmptyState, ErrorText, Resource, StatusBadge, StatusText } from "../../components/status";
import { useAction, useResource } from "../../lib/use-resource";
import { formatDateTime } from "../../lib/format";

const PAGE_SIZE = 20;

const fetchPage = (cursor?: string, signal?: AbortSignal) =>
    listNotifications({ limit: PAGE_SIZE, cursor, signal }).then(({ data, next_cursor }) => ({
        items: data,
        cursor: next_cursor
    }));

const loadFirstPage = (signal: AbortSignal) => fetchPage(undefined, signal);

const loadPreferences = (signal: AbortSignal) =>
    getNotificationPreferences({ signal }).then(({ data }) => data);

export default function NotificationsView() {
    return (
        <AccountPage title="通知設定" description="選擇想收到哪些提醒，以及要寄到哪裡。">
            <PreferencesForm />
            <NotificationList />
        </AccountPage>
    );
}

function NotificationList() {
    const { state, reload, setData } = useResource(loadFirstPage);
    const { pending, error: actionError, run } = useAction();

    const markLocally = (match: (item: Notification) => boolean) => {
        const readAt = new Date().toISOString();
        setData((page) => ({
            ...page,
            items: page.items.map((item) =>
                !item.read_at && match(item) ? { ...item, read_at: readAt } : item
            )
        }));
    };

    const loadMore = (cursor: string) =>
        run("more", async () => {
            const next = await fetchPage(cursor);
            setData((page) => ({ items: [...page.items, ...next.items], cursor: next.cursor }));
        });

    const hasUnread = state.status === "ready" && state.data.items.some((item) => !item.read_at);

    return (
        <Card
            title="最近的通知"
            className="flex flex-col gap-5"
            actions={
                hasUnread && (
                    <Button
                        variant="secondary"
                        size="sm"
                        className="gap-2 text-base"
                        disabled={pending !== null}
                        onClick={() =>
                            run("all", async () => {
                                await markAllNotificationsRead();
                                markLocally(() => true);
                            })
                        }
                    >
                        <CheckCheck aria-hidden className="size-4" />
                        全部標為已讀
                    </Button>
                )
            }
        >
            <ErrorText>{actionError}</ErrorText>
            <Resource state={state} reload={reload}>
                {(page) =>
                    page.items.length === 0 ? (
                        <EmptyState
                            title="目前沒有通知"
                            description="有新的查榜提醒或客服回覆時會顯示在這裡。"
                        />
                    ) : (
                        <>
                            <ul className="flex flex-col divide-y divide-ink/10">
                                {page.items.map((item) => {
                                    const unread = !item.read_at;
                                    return (
                                        <li
                                            key={item.id}
                                            className={twMerge(
                                                "flex flex-col gap-3 py-4 sm:flex-row sm:items-start sm:gap-4",
                                                unread &&
                                                    "-mx-3 rounded-[var(--radius-small)] bg-accent-yellow-light/60 px-3"
                                            )}
                                        >
                                            <Bell
                                                aria-hidden
                                                className={twMerge(
                                                    "hidden size-5 shrink-0 sm:mt-1 sm:block",
                                                    unread ? "text-ink" : "text-copy-muted"
                                                )}
                                            />
                                            <div className="min-w-0 flex-1">
                                                <div className="flex flex-wrap items-center gap-2">
                                                    {unread && (
                                                        <StatusBadge tone="pending">
                                                            未讀
                                                        </StatusBadge>
                                                    )}
                                                    <StatusBadge>
                                                        {notificationKindLabel(item.kind)}
                                                    </StatusBadge>
                                                    <time
                                                        dateTime={item.created_at}
                                                        className="font-sans text-sm text-copy-muted"
                                                    >
                                                        {formatDateTime(item.created_at)}
                                                    </time>
                                                </div>
                                                <h3
                                                    className={twMerge(
                                                        "mt-2 font-sans text-lg text-ink",
                                                        unread ? "font-semibold" : "font-medium"
                                                    )}
                                                >
                                                    {item.title}
                                                </h3>
                                                <p className="mt-1 font-sans text-base leading-relaxed break-words whitespace-pre-line text-copy-muted">
                                                    {item.body}
                                                </p>
                                            </div>
                                            {unread && (
                                                <Button
                                                    variant="secondary"
                                                    size="sm"
                                                    className="self-start"
                                                    disabled={pending !== null}
                                                    onClick={() =>
                                                        run(item.id, async () => {
                                                            await markNotificationRead(item.id);
                                                            markLocally(
                                                                (other) => other.id === item.id
                                                            );
                                                        })
                                                    }
                                                >
                                                    標為已讀
                                                </Button>
                                            )}
                                        </li>
                                    );
                                })}
                            </ul>
                            {page.cursor && (
                                <Button
                                    variant="secondary"
                                    size="base"
                                    className="self-center"
                                    disabled={pending !== null}
                                    onClick={() => loadMore(page.cursor)}
                                >
                                    {pending === "more" ? "載入中…" : "載入更多"}
                                </Button>
                            )}
                        </>
                    )
                }
            </Resource>
        </Card>
    );
}

function PreferencesForm() {
    const { state, reload } = useResource(loadPreferences);

    return (
        <Card title="通知偏好" className="flex flex-col gap-5">
            <Resource state={state} reload={reload}>
                {(preferences) => <PreferencesEditor initial={preferences} />}
            </Resource>
        </Card>
    );
}

function PreferencesEditor({ initial }: { initial: NotificationPreferences }) {
    const [preferences, setPreferences] = useState(initial);
    const { pending, error, run } = useAction();
    const [saved, setSaved] = useState(false);

    const update = (next: (current: NotificationPreferences) => NotificationPreferences) => {
        setSaved(false);
        setPreferences(next);
    };
    const setKind = (kind: string, checked: boolean) =>
        update((current) => ({ ...current, kinds: { ...current.kinds, [kind]: checked } }));

    async function save(event: React.FormEvent) {
        event.preventDefault();
        setSaved(false);
        setSaved(
            await run("save", async () => {
                const { data } = await updateNotificationPreferences(preferences);
                setPreferences(data);
            })
        );
    }

    return (
        <form onSubmit={save} className="mt-5 flex flex-col gap-5">
            <FieldGroup label="Email 通知">
                <p className="-mt-1 font-sans text-sm text-copy-muted">
                    站內通知仍會顯示在下方；關閉後只會停止一般通知信，帳號安全信件不受影響。
                </p>
                <Checkbox
                    label="寄送一般通知至聯絡信箱"
                    checked={preferences.email_enabled}
                    onCheckedChange={(checked) =>
                        update((current) => ({ ...current, email_enabled: checked }))
                    }
                />
            </FieldGroup>
            <FieldGroup label="通知項目">
                {NOTIFICATION_KINDS.map(({ kind, label, description }) => (
                    <div key={kind} className="flex flex-col gap-1">
                        <Checkbox
                            label={label}
                            checked={preferences.kinds[kind] ?? true}
                            onCheckedChange={(checked) => setKind(kind, checked)}
                        />
                        <p className="pl-8 font-sans text-sm text-copy-muted">{description}</p>
                    </div>
                ))}
            </FieldGroup>
            <div className="flex flex-col gap-3 pt-5 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex flex-col gap-1">
                    <StatusText>{saved ? "已儲存通知偏好。" : null}</StatusText>
                    <ErrorText>{error}</ErrorText>
                </div>
                <div className="flex flex-col-reverse gap-3 sm:flex-row">
                    <Button type="submit" size="base" disabled={pending !== null}>
                        {pending ? "儲存中…" : "儲存"}
                    </Button>
                </div>
            </div>
        </form>
    );
}
