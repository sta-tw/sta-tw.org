"use client";

import Link from "next/link";
import { Bell, CheckCheck } from "lucide-react";
import { useState } from "react";
import { twMerge } from "tailwind-merge";
import Button from "../../components/button";
import { Card } from "../../components/card";
import Checkbox from "../../components/checkbox";
import { listLinkedIdentities } from "../../lib/api/identities";
import {
    DEFAULT_NOTIFICATION_PREFERENCES,
    getNotificationPreferences,
    listNotifications,
    markAllNotificationsRead,
    markNotificationRead,
    NOTIFICATION_KINDS,
    notificationKindLabel,
    updateNotificationPreferences,
    type Notification,
    type NotificationChannel,
    type NotificationGroup,
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
    Promise.all([
        getNotificationPreferences({ signal }),
        // Only decides whether the Discord channel can be turned on.
        listLinkedIdentities({ signal }).then(
            ({ data }) => data.some((identity) => identity.provider === "discord"),
            () => false
        )
    ]).then(([{ data }, discordLinked]) => ({ preferences: data, discordLinked }));

const kindGroups: { group: NotificationGroup; title: string }[] = [
    { group: "personal", title: "通知項目 - 個人" },
    { group: "interaction", title: "通知項目 - 互動" }
];

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
                {({ preferences, discordLinked }) => (
                    <PreferencesEditor initial={preferences} discordLinked={discordLinked} />
                )}
            </Resource>
        </Card>
    );
}

// Each fieldset sits in a div that carries the divider: a border on the
// fieldset itself would run through its legend.
const sectionClass = "py-5 first:pt-0";

function PreferencesEditor({
    initial,
    discordLinked
}: {
    initial: NotificationPreferences;
    discordLinked: boolean;
}) {
    const [preferences, setPreferences] = useState(initial);
    const { pending, error, run } = useAction();
    const [saved, setSaved] = useState(false);

    const update = (next: (current: NotificationPreferences) => NotificationPreferences) => {
        setSaved(false);
        setPreferences(next);
    };
    const setChannel = (channel: NotificationChannel, checked: boolean) =>
        update((current) => ({
            ...current,
            channels: { ...current.channels, [channel]: checked }
        }));

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
        <form onSubmit={save} className="mt-5 flex flex-col divide-y divide-ink/10">
            <div className={sectionClass}>
                <FieldGroup label="通知管道">
                    <p className="-mt-1 font-sans text-sm text-copy-muted">
                        站內通知一律會顯示在下方；勾選後也會另外寄送。帳號安全信件（驗證、密碼重設）不受影響。
                    </p>
                    <Checkbox
                        label="Email（寄到你的聯絡信箱）"
                        checked={preferences.channels.email}
                        onCheckedChange={(checked) => setChannel("email", checked)}
                    />
                    <div className="flex flex-col gap-1">
                        <Checkbox
                            label="Discord（由特選群機器人私訊）"
                            checked={discordLinked && preferences.channels.discord}
                            disabled={!discordLinked}
                            onCheckedChange={(checked) => setChannel("discord", checked)}
                        />
                        {!discordLinked && (
                            <p className="pl-8 font-sans text-sm text-copy-muted">
                                需先
                                <Link
                                    href="/account/verification"
                                    className="mx-1 text-ink underline underline-offset-4"
                                >
                                    綁定 Discord 帳號
                                </Link>
                                ，並加入特選群。
                            </p>
                        )}
                    </div>
                </FieldGroup>
            </div>
            {kindGroups.map(({ group, title }) => (
                <div key={group} className={sectionClass}>
                    <FieldGroup label={title}>
                        {NOTIFICATION_KINDS.filter((item) => item.group === group).map(
                            ({ kind, label }) => (
                                <Checkbox
                                    key={kind}
                                    label={label}
                                    checked={preferences.kinds[kind] ?? false}
                                    onCheckedChange={(checked) =>
                                        update((current) => ({
                                            ...current,
                                            kinds: { ...current.kinds, [kind]: checked }
                                        }))
                                    }
                                />
                            )
                        )}
                    </FieldGroup>
                </div>
            ))}
            <div className="flex flex-col gap-3 pt-5 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex flex-col gap-1">
                    <StatusText>{saved ? "已儲存通知偏好。" : null}</StatusText>
                    <ErrorText>{error}</ErrorText>
                </div>
                <div className="flex flex-col-reverse gap-3 sm:flex-row">
                    <Button
                        type="button"
                        variant="secondary"
                        size="base"
                        disabled={pending !== null}
                        onClick={() => update(() => DEFAULT_NOTIFICATION_PREFERENCES)}
                    >
                        恢復預設值
                    </Button>
                    <Button type="submit" size="base" disabled={pending !== null}>
                        {pending ? "儲存中…" : "儲存"}
                    </Button>
                </div>
            </div>
        </form>
    );
}
