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
    updateNotificationPreferences,
    type Notification,
    type NotificationPreferences
} from "../../lib/api/notifications";
import AccountPage from "../account-page";
import { FieldGroup } from "../../components/field";
import { EmptyState, ErrorText, Resource, SaveBar, StatusBadge } from "../../components/status";
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

const kindLabel = (kind: string) =>
    NOTIFICATION_KINDS.find((item) => item.kind === kind)?.label ?? "系統通知";

export default function NotificationsView() {
    return (
        <AccountPage title="通知設定" description="查看站內通知，並選擇想收到哪些提醒。">
            <NotificationList />
            <PreferencesForm />
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
            title="站內通知"
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
                                                        {kindLabel(item.kind)}
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
                {(initial) => <PreferencesEditor initial={initial} />}
            </Resource>
        </Card>
    );
}

function PreferencesEditor({ initial }: { initial: NotificationPreferences }) {
    const [preferences, setPreferences] = useState(initial);
    const { pending, error, run } = useAction();
    const [saved, setSaved] = useState(false);

    const kindEnabled = (kind: string) => preferences.kinds[kind] ?? true;

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
        <form onSubmit={save} className="flex flex-col gap-6">
            <FieldGroup label="通知類型" className="gap-4">
                {NOTIFICATION_KINDS.map(({ kind, label, description }) => (
                    <div key={kind} className="flex flex-col gap-1">
                        <Checkbox
                            label={label}
                            checked={kindEnabled(kind)}
                            onCheckedChange={(checked) =>
                                setPreferences((current) => ({
                                    ...current,
                                    kinds: { ...current.kinds, [kind]: checked }
                                }))
                            }
                        />
                        <p className="pl-8 font-sans text-sm text-copy-muted">{description}</p>
                    </div>
                ))}
            </FieldGroup>
            <fieldset className="flex flex-col gap-1 border-t border-ink/10 pt-5">
                <legend className="sr-only">寄送方式</legend>
                <Checkbox
                    label="同時以 Email 寄送通知"
                    checked={preferences.email_enabled}
                    onCheckedChange={(checked) =>
                        setPreferences((current) => ({ ...current, email_enabled: checked }))
                    }
                />
                <p className="pl-8 font-sans text-sm text-copy-muted">
                    驗證信、密碼重設等帳號安全信件不受此設定影響，一律會寄出。
                </p>
            </fieldset>
            <SaveBar
                pending={pending !== null}
                saved={saved}
                error={error}
                label="儲存設定"
                savedText="已儲存通知偏好。"
            />
        </form>
    );
}
