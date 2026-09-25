"use client";

import { useEffect, useMemo, useState } from "react";
import {
    Award,
    CalendarPlus,
    CalendarSearch,
    CircleCheck,
    CircleX,
    ClipboardPenLine,
    Download,
    ExternalLink,
    Heart
} from "lucide-react";
import { Tabs } from "radix-ui";
import type { Brochure, BrochureFact, BrochureTimelineItem } from "../lib/brochure-types";
import type { OpenGraphPreview } from "../lib/open-graph";
import {
    createCalendarEvents,
    getCalendarStatus,
    removeCalendarEvent,
    startCalendarLink,
    type CalendarEventInput
} from "../lib/api/calendar";
import {
    getPublishedBrochureDownload,
    listPublishedBrochures,
    type BrochureDocument
} from "../lib/api/admissions";
import { ApiError } from "../lib/api/types";

// A pending batch survives the redirect to Google and back (the page fully
// reloads on that round trip, so React state can't carry it) — stashed here
// right before navigating away, consumed once by the oauth=success effect
// below.
const PENDING_CALENDAR_EVENTS_KEY = "sta-pending-calendar-events";

function stashPendingCalendarEvents(events: CalendarEventInput[]) {
    try {
        sessionStorage.setItem(PENDING_CALENDAR_EVENTS_KEY, JSON.stringify(events));
    } catch {
        // Storage can throw in a private window with blocked site data —
        // the user just has to click the button again after linking, so
        // this is a convenience, not a correctness requirement.
    }
}

function takePendingCalendarEvents(): CalendarEventInput[] | null {
    try {
        const raw = sessionStorage.getItem(PENDING_CALENDAR_EVENTS_KEY);
        sessionStorage.removeItem(PENDING_CALENDAR_EVENTS_KEY);
        return raw ? (JSON.parse(raw) as CalendarEventInput[]) : null;
    } catch {
        return null;
    }
}

type BrochureDetailTabsProps = {
    brochure: Brochure;
    externalLinkPreviews: OpenGraphPreview[];
    downloadUrl?: string;
    schoolCode: string;
};

const tabs = [
    { value: "overview", label: "招生資訊" },
    { value: "about", label: "關於學系" },
    { value: "registration", label: "報名資訊" },
    { value: "history", label: "歷年招生資料" }
];

export default function BrochureDetailTabs({
    brochure,
    externalLinkPreviews,
    downloadUrl,
    schoolCode
}: BrochureDetailTabsProps) {
    return (
        <Tabs.Root defaultValue="overview" className="mt-7 sm:mt-9">
            <div className="-mx-5 overflow-x-auto px-5 pb-2 sm:mx-0 sm:px-0">
                <Tabs.List
                    aria-label="簡章內容"
                    className="flex w-max min-w-full gap-2 border-b border-ink/15 pb-3 sm:gap-3"
                >
                    {tabs.map((tab) => (
                        <Tabs.Trigger
                            key={tab.value}
                            value={tab.value}
                            className="cursor-pointer rounded-[var(--radius-small)] bg-accent-green/45 px-3 py-2 font-sans text-sm font-medium whitespace-nowrap text-ink transition-colors hover:bg-accent-green/80 focus-visible:ring-2 focus-visible:ring-ink focus-visible:outline-none data-[state=active]:bg-accent-green-strong sm:text-base"
                        >
                            {tab.label}
                        </Tabs.Trigger>
                    ))}
                </Tabs.List>
            </div>

            <div className="mt-4 flex flex-col gap-3 sm:mt-5 sm:flex-row sm:flex-wrap sm:items-center">
                <p className="w-fit rounded-[var(--radius-small)] bg-accent-yellow px-4 py-2 font-sans text-sm leading-snug text-ink sm:text-base">
                    資料以後端已審核的招生資料為準
                </p>
                <div className="flex flex-wrap gap-2">
                    {downloadUrl ? (
                        <a
                            href={downloadUrl}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="inline-flex h-10 shrink-0 cursor-pointer items-center gap-2 rounded-full bg-accent-yellow/75 px-3 font-sans text-sm font-medium whitespace-nowrap text-ink transition-colors hover:bg-accent-yellow focus-visible:ring-2 focus-visible:ring-ink focus-visible:ring-offset-2 focus-visible:outline-none"
                        >
                            <Download aria-hidden className="h-4 w-4 shrink-0" />
                            簡章下載
                        </a>
                    ) : (
                        <button
                            type="button"
                            disabled
                            title="目前沒有已上架的簡章檔案"
                            className="inline-flex h-10 shrink-0 items-center gap-2 rounded-full bg-ink/10 px-3 font-sans text-sm font-medium whitespace-nowrap text-ink/50"
                        >
                            <Download aria-hidden className="h-4 w-4 shrink-0" />
                            尚未提供下載
                        </button>
                    )}
                    <button
                        type="button"
                        className="inline-flex h-10 shrink-0 cursor-pointer items-center gap-2 rounded-full bg-accent-yellow/75 px-3 font-sans text-sm font-medium whitespace-nowrap text-ink transition-colors hover:bg-accent-yellow focus-visible:ring-2 focus-visible:ring-ink focus-visible:ring-offset-2 focus-visible:outline-none"
                    >
                        <Heart aria-hidden className="h-4 w-4 shrink-0" />
                        我對此學系有興趣
                    </button>
                </div>
            </div>

            <Tabs.Content value="overview" className="outline-none">
                <OverviewTab brochure={brochure} />
            </Tabs.Content>
            <Tabs.Content value="about" className="outline-none">
                <ExternalLinksTab brochure={brochure} previews={externalLinkPreviews} />
            </Tabs.Content>
            <Tabs.Content value="registration" className="outline-none">
                <RegistrationTab brochure={brochure} />
            </Tabs.Content>
            <Tabs.Content value="history" className="outline-none">
                <HistoryTab brochure={brochure} schoolCode={schoolCode} />
            </Tabs.Content>
        </Tabs.Root>
    );
}

function OverviewTab({ brochure }: Pick<BrochureDetailTabsProps, "brochure">) {
    return (
        <div className="mt-8 space-y-7 sm:mt-10 sm:space-y-8">
            <dl className="flex flex-wrap gap-x-8 gap-y-4 sm:gap-x-10">
                {brochure.facts.map((fact) => (
                    <Fact key={fact.label} {...fact} compact />
                ))}
            </dl>
            <dl className="space-y-5 sm:space-y-6">
                <Fact label="報名資格" value={brochure.eligibility} />
                <Fact label="考試方式" value={brochure.examFormat} />
                <Fact label="報名費用" value={brochure.fee} />
                <Fact label="報名時程" value={brochure.timeline} />
            </dl>
        </div>
    );
}

function ExternalLinksTab({
    brochure,
    previews
}: Pick<BrochureDetailTabsProps, "brochure"> & { previews: OpenGraphPreview[] }) {
    const pinnedLinks = [
        brochure.schoolOfficialUrl ? { label: "學校官網", url: brochure.schoolOfficialUrl } : null,
        brochure.departmentOfficialUrl
            ? { label: "系網", url: brochure.departmentOfficialUrl }
            : null
    ].filter((link): link is { label: string; url: string } => link !== null);

    if (pinnedLinks.length === 0 && previews.length === 0) {
        return (
            <div className="mt-8 rounded-[var(--radius-panel)] border border-dashed border-ink/20 bg-surface/55 px-5 py-10 text-center sm:mt-10">
                <p className="font-sans text-base text-ink/65">目前沒有可用的相關連結。</p>
            </div>
        );
    }

    return (
        <div className="mt-8 flex flex-col gap-6 sm:mt-10">
            {pinnedLinks.length > 0 ? (
                <ul className="flex flex-wrap gap-2">
                    {pinnedLinks.map((link) => (
                        <li key={link.label}>
                            <a
                                href={link.url}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="inline-flex h-10 shrink-0 cursor-pointer items-center gap-2 rounded-full bg-accent-green-strong px-4 font-sans text-sm font-medium whitespace-nowrap text-ink transition-colors hover:bg-accent-green focus-visible:ring-2 focus-visible:ring-ink focus-visible:ring-offset-2 focus-visible:outline-none"
                            >
                                {link.label}
                                <ExternalLink aria-hidden className="h-4 w-4 shrink-0" />
                            </a>
                        </li>
                    ))}
                </ul>
            ) : null}
            {previews.length > 0 ? (
                <ul className="space-y-4 sm:space-y-5">
                    {previews.map((preview) => (
                        <li key={preview.url}>
                            <a
                                href={preview.url}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="group grid overflow-hidden rounded-[var(--radius-panel)] border border-ink/10 bg-surface/80 transition-[border-color,box-shadow,transform] hover:-translate-y-0.5 hover:border-ink/25 hover:shadow-[var(--shadow-card)] focus-visible:ring-2 focus-visible:ring-ink focus-visible:ring-offset-2 focus-visible:outline-none sm:grid-cols-[14rem_minmax(0,1fr)]"
                            >
                                <div
                                    aria-hidden
                                    className="min-h-40 bg-accent-green/60 bg-cover bg-center sm:min-h-full"
                                    style={
                                        preview.imageUrl
                                            ? { backgroundImage: `url(\"${preview.imageUrl}\")` }
                                            : undefined
                                    }
                                />
                                <div className="min-w-0 p-5 sm:p-6">
                                    <p className="font-sans text-sm text-ink/55">{preview.siteName}</p>
                                    <h2 className="mt-2 flex items-start gap-2 font-sans text-xl leading-snug font-medium text-ink sm:text-2xl">
                                        <span>{preview.title}</span>
                                        <ExternalLink
                                            aria-hidden
                                            className="mt-1 h-4 w-4 shrink-0 text-ink/55"
                                        />
                                    </h2>
                                    {preview.description && (
                                        <p className="mt-4 line-clamp-3 font-sans text-base leading-7 text-ink/70">
                                            {preview.description}
                                        </p>
                                    )}
                                </div>
                            </a>
                        </li>
                    ))}
                </ul>
            ) : null}
        </div>
    );
}

function RegistrationTab({ brochure }: Pick<BrochureDetailTabsProps, "brochure">) {
    const items = brochure.registrationTimeline;
    const [checked, setChecked] = useState<Record<string, boolean>>({});
    const [busy, setBusy] = useState(false);
    const [feedback, setFeedback] = useState<{ tone: "success" | "error" | "info"; text: string } | null>(null);
    // Which of this programme's timeline items already have a Google
    // Calendar event on file for this account — drives "已加入 / 移除"
    // instead of the add checkbox for items already there.
    const [linkedItemIds, setLinkedItemIds] = useState<Set<string>>(new Set());
    const [removingId, setRemovingId] = useState<string | null>(null);
    const temporalStates = useMemo(() => computeTimelineTemporalStates(items), [items]);

    function externalId(item: BrochureTimelineItem) {
        return `${brochure.slug}:${item.id}`;
    }

    function refreshLinkedStatus() {
        getCalendarStatus()
            .then((status) => {
                const prefix = `${brochure.slug}:`;
                const linked = (status.linked_event_ids ?? [])
                    .filter((id) => id.startsWith(prefix))
                    .map((id) => id.slice(prefix.length));
                setLinkedItemIds(new Set(linked));
            })
            .catch(() => {
                // Not logged in, or the check failed — just leave nothing
                // marked as linked; the add flow's own 401/428 handling
                // covers those cases when the visitor actually tries to add.
            });
    }

    // Deferred a tick, same reasoning as the oauth=success effect below:
    // setState inside refreshLinkedStatus's .then() must not run synchronously
    // within this effect's own commit.
    useEffect(() => {
        void Promise.resolve().then(refreshLinkedStatus);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [brochure.slug]);

    function toggle(id: string) {
        setChecked((prev) => ({ ...prev, [id]: !prev[id] }));
    }

    function buildEvents(): CalendarEventInput[] {
        const checkedIds = Object.keys(checked).filter((id) => checked[id]);
        const selected = items.filter(
            (item) =>
                !linkedItemIds.has(item.id) &&
                (checkedIds.length > 0 ? checkedIds.includes(item.id) : true) &&
                item.isoStart
        );
        const details = `${brochure.university} ${brochure.department}\n${brochure.externalLinks[0]?.url ?? ""}`;
        return selected.map((item) => ({
            title: `${brochure.university} ${brochure.department} - ${item.title}`,
            iso_start: item.isoStart!,
            iso_end: item.isoEnd ?? item.isoStart!,
            details,
            external_id: externalId(item)
        }));
    }

    async function submitEvents(events: CalendarEventInput[]) {
        setBusy(true);
        setFeedback(null);
        try {
            const result = await createCalendarEvents(events);
            setFeedback({ tone: "success", text: `已新增 ${result.created} 筆到你的 Google 日曆。` });
            if (result.created === events.length) {
                const prefix = `${brochure.slug}:`;
                setLinkedItemIds((prev) => {
                    const next = new Set(prev);
                    for (const event of events) {
                        if (event.external_id?.startsWith(prefix)) {
                            next.add(event.external_id.slice(prefix.length));
                        }
                    }
                    return next;
                });
            } else {
                // A partial batch: we don't know which ones actually landed,
                // so re-fetch the real state from the server instead of
                // guessing at it.
                refreshLinkedStatus();
            }
        } catch (cause) {
            if (cause instanceof ApiError && cause.status === 401) {
                setFeedback({ tone: "info", text: "請先登入 S.T.A 帳號，才能使用「新增至日曆」。" });
                return;
            }
            if (cause instanceof ApiError && cause.status === 428) {
                stashPendingCalendarEvents(events);
                try {
                    const returnTo = `${window.location.pathname}${window.location.search}`;
                    const { authorization_url } = await startCalendarLink(returnTo);
                    window.location.href = authorization_url;
                } catch {
                    setFeedback({ tone: "error", text: "無法啟動 Google 授權流程，請稍後再試。" });
                }
                return;
            }
            setFeedback({ tone: "error", text: "新增至日曆失敗，請稍後再試。" });
        } finally {
            setBusy(false);
        }
    }

    async function removeEvent(item: BrochureTimelineItem) {
        setRemovingId(item.id);
        setFeedback(null);
        try {
            await removeCalendarEvent(externalId(item));
            setLinkedItemIds((prev) => {
                const next = new Set(prev);
                next.delete(item.id);
                return next;
            });
            setChecked((prev) => ({ ...prev, [item.id]: false }));
            setFeedback({ tone: "success", text: "已從你的 Google 日曆移除。" });
        } catch (cause) {
            if (cause instanceof ApiError && cause.status === 404) {
                // Already gone (removed by hand in Google Calendar, say) —
                // just drop the stale "linked" state locally too.
                setLinkedItemIds((prev) => {
                    const next = new Set(prev);
                    next.delete(item.id);
                    return next;
                });
                return;
            }
            setFeedback({ tone: "error", text: "移除失敗，請稍後再試。" });
        } finally {
            setRemovingId(null);
        }
    }

    // A visitor who just came back from the Google consent screen has a
    // pending batch stashed before the redirect; this page reloaded fresh
    // (the OAuth round trip is a full navigation), so re-check status and
    // resubmit automatically instead of making them click the button again.
    useEffect(() => {
        if (new URLSearchParams(window.location.search).get("oauth") !== "success") return;
        const pending = takePendingCalendarEvents();
        if (!pending || pending.length === 0) return;
        // Deferred a tick so the setState calls inside submitEvents don't
        // run synchronously within this effect's own commit.
        void Promise.resolve().then(() => submitEvents(pending));
    }, []);

    function handleAddToCalendar() {
        const events = buildEvents();
        if (events.length === 0) return;
        void submitEvents(events);
    }

    return (
        <section className="mt-8 max-w-2xl sm:mt-10" aria-labelledby="admission-timeline-title">
            <div className="flex flex-col gap-4 border-b border-ink/10 pb-5 sm:flex-row sm:items-center sm:justify-between">
                <div>
                    <h2
                        id="admission-timeline-title"
                        className="font-sans text-xl font-medium text-ink sm:text-2xl"
                    >
                        招生時程
                    </h2>
                    <p className="mt-1 font-sans text-sm text-ink/60">
                        勾選重要日期後點「新增至日曆」，會直接寫入你的 Google
                        日曆（未勾選則加入全部）；已加入的項目可以點「移除」從日曆刪掉。第一次使用需要用
                        Google 帳號授權一次，之後就不用再問。
                    </p>
                </div>
                <button
                    type="button"
                    onClick={handleAddToCalendar}
                    disabled={busy}
                    className="inline-flex h-10 w-fit shrink-0 cursor-pointer items-center gap-2 rounded-full bg-accent-yellow px-3.5 font-sans text-sm font-medium whitespace-nowrap text-ink transition-colors hover:bg-[#f6bd42] focus-visible:ring-2 focus-visible:ring-ink focus-visible:ring-offset-2 focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-60"
                >
                    <CalendarPlus aria-hidden className="h-4 w-4 shrink-0" />
                    {busy ? "處理中…" : "新增至日曆"}
                </button>
            </div>

            {feedback && (
                <p
                    className={`mt-3 font-sans text-sm ${
                        feedback.tone === "success"
                            ? "text-accent-green-strong"
                            : feedback.tone === "error"
                              ? "text-red-600"
                              : "text-ink/70"
                    }`}
                    role="status"
                >
                    {feedback.text}
                </p>
            )}

            <ol className="relative mt-7 ml-10 border-l border-ink/20 sm:mt-9 sm:ml-12">
                {items.map((item) => (
                    <TimelineItem
                        key={item.id}
                        item={item}
                        checked={Boolean(checked[item.id])}
                        onToggle={() => toggle(item.id)}
                        linked={linkedItemIds.has(item.id)}
                        removing={removingId === item.id}
                        onRemove={() => void removeEvent(item)}
                        temporalState={temporalStates.get(item.id) ?? "future"}
                    />
                ))}
            </ol>
        </section>
    );
}

// Each item is colored relative to today rather than by what kind of
// milestone it is: white/gray for something already over, yellow for
// whichever not-yet-over item is happening/coming up soonest (the "current"
// stage — there's only ever one), green for everything further out. An
// item with no resolvable date can't be placed in time, so it defaults to
// "future" rather than implying it's already done.
type TimelineTemporalState = "past" | "current" | "future";

const timelineTemporalClassName: Record<TimelineTemporalState, string> = {
    past: "border-ink/25 bg-surface text-ink/70",
    current: "border-[#f6bd42] bg-accent-yellow text-ink",
    future: "border-accent-green-strong bg-accent-green text-ink"
};

// The icon, unlike the color, is about *what kind* of milestone this is
// rather than when it falls — a past registration deadline and a past
// interview date are both "over", but they're different kinds of "over".
// A past item always gets the same dedicated icon regardless of category,
// since by then which stage it belonged to matters less than "this part is
// done"; a not-yet-over item's icon depends on which of the three stages a
// brochure's schedule naturally falls into.
type TimelineCategory = "registration" | "assessment" | "admission";

// Checked in this order (most specific first) since a title can contain
// more than one keyword family — e.g. "成績複查申請期限" mentions 成績 (an
// assessment-flavored word) but is really a post-admission administrative
// step, so admission keywords are matched before assessment ones.
const ADMISSION_CATEGORY_KEYWORDS = [
    "放榜",
    "錄取",
    "正取",
    "備取",
    "報到",
    "遞補",
    "放棄",
    "資格",
    "成績複查"
];
const ASSESSMENT_CATEGORY_KEYWORDS = ["複試", "初試", "面試", "筆試", "口試", "名單", "術科", "考試"];

function classifyTimelineCategory(title: string): TimelineCategory {
    if (ADMISSION_CATEGORY_KEYWORDS.some((keyword) => title.includes(keyword))) return "admission";
    if (ASSESSMENT_CATEGORY_KEYWORDS.some((keyword) => title.includes(keyword))) return "assessment";
    return "registration";
}

const timelineCategoryIcon: Record<TimelineCategory, typeof Award> = {
    registration: ClipboardPenLine,
    assessment: CalendarSearch,
    admission: Award
};

function computeTimelineTemporalStates(
    items: BrochureTimelineItem[]
): Map<string, TimelineTemporalState> {
    const todayISO = new Date().toISOString().slice(0, 10);
    const states = new Map<string, TimelineTemporalState>();
    let currentId: string | null = null;
    let currentStart: string | null = null;
    for (const item of items) {
        const end = item.isoEnd ?? item.isoStart;
        if (!end) continue;
        if (end < todayISO) {
            states.set(item.id, "past");
            continue;
        }
        states.set(item.id, "future");
        // An already-ongoing range (isoStart in the past, isoEnd not yet)
        // has the smallest isoStart of any not-yet-over item, so it always
        // wins this comparison and correctly becomes "current" ahead of
        // anything that hasn't started yet.
        const start = item.isoStart ?? end;
        if (currentStart === null || start < currentStart) {
            currentStart = start;
            currentId = item.id;
        }
    }
    if (currentId) states.set(currentId, "current");
    return states;
}

function TimelineItem({
    item,
    checked,
    onToggle,
    linked,
    removing,
    onRemove,
    temporalState
}: {
    item: BrochureTimelineItem;
    checked: boolean;
    onToggle: () => void;
    linked: boolean;
    removing: boolean;
    onRemove: () => void;
    temporalState: TimelineTemporalState;
}) {
    const className = timelineTemporalClassName[temporalState];
    const Icon =
        temporalState === "past" ? CircleCheck : timelineCategoryIcon[classifyTimelineCategory(item.title)];

    return (
        <li className="relative pb-7 pl-8 last:pb-0 sm:pb-8 sm:pl-10">
            {linked ? (
                <button
                    type="button"
                    onClick={onRemove}
                    disabled={removing}
                    aria-label={`從日曆移除 ${item.title}`}
                    title="從 Google 日曆移除"
                    className="absolute top-3 -left-[3.3rem] flex h-4 w-4 cursor-pointer items-center justify-center text-ink/50 transition-colors hover:text-red-600 disabled:cursor-not-allowed disabled:opacity-40 sm:-left-[3.8rem]"
                >
                    <CircleX aria-hidden className="h-4 w-4" />
                </button>
            ) : (
                <input
                    id={`timeline-${item.id}`}
                    type="checkbox"
                    name="calendar-events"
                    value={item.id}
                    checked={checked}
                    onChange={onToggle}
                    disabled={!item.isoStart}
                    className="absolute top-3 -left-[3.3rem] h-4 w-4 cursor-pointer accent-button focus-visible:ring-2 focus-visible:ring-ink focus-visible:ring-offset-2 focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-40 sm:-left-[3.8rem]"
                />
            )}
            <label
                htmlFor={`timeline-${item.id}`}
                className="absolute top-0 -left-5 flex h-10 w-10 cursor-pointer items-center justify-center rounded-full border bg-surface sm:-left-6 sm:h-12 sm:w-12"
            >
                <span
                    className={`flex h-8 w-8 items-center justify-center rounded-full ${className}`}
                >
                    <Icon aria-hidden className="h-5 w-5" />
                </span>
                <span className="sr-only">選擇 {item.title}</span>
            </label>
            <div className="min-w-0 pt-1.5 sm:pt-2">
                <h3 className="font-sans text-base leading-snug font-medium text-ink sm:text-lg">
                    {item.title}
                </h3>
                <p className="mt-1 font-sans text-sm leading-relaxed text-ink/70 sm:text-base">
                    {item.date}
                </p>
                {linked && (
                    <p className="mt-1 font-sans text-xs text-accent-green-strong">
                        {removing ? "移除中…" : "已加入 Google 日曆"}
                    </p>
                )}
            </div>
        </li>
    );
}

function HistoryTab({
    brochure,
    schoolCode
}: Pick<BrochureDetailTabsProps, "brochure" | "schoolCode">) {
    return (
        <div className="mt-8 flex flex-col gap-8 sm:mt-10">
            {brochure.history.length === 0 ? (
                <div className="rounded-[var(--radius-panel)] border border-dashed border-ink/20 bg-surface/55 px-5 py-10 text-center">
                    <p className="font-sans text-base text-ink/65">目前沒有可公開的歷年招生資料。</p>
                </div>
            ) : (
                <div className="overflow-x-auto rounded-[var(--radius-small)] border border-ink/15 bg-surface/65">
                    <table className="w-full min-w-180 border-collapse text-center font-sans text-sm sm:text-base">
                        <caption className="sr-only">歷年招生資料</caption>
                        <thead className="bg-ink/3 font-medium text-ink">
                            <tr className="divide-x divide-ink/15">
                                <th scope="col" className="px-4 py-3 font-medium">
                                    年份
                                </th>
                                <th scope="col" className="px-4 py-3 font-medium">
                                    正取人數
                                </th>
                                <th scope="col" className="px-4 py-3 font-medium">
                                    遞補人數
                                </th>
                                <th scope="col" className="px-4 py-3 font-medium">
                                    備取人數
                                </th>
                                <th scope="col" className="px-4 py-3 font-medium">
                                    報名人數
                                </th>
                            </tr>
                        </thead>
                        <tbody className="divide-y divide-ink/15 text-ink/75">
                            {brochure.history.map((row) => (
                                <tr key={row.year} className="divide-x divide-ink/15">
                                    <td className="px-4 py-3">{row.year}</td>
                                    <td className="px-4 py-3">{row.admitted}</td>
                                    <td className="px-4 py-3">{row.waitlisted}</td>
                                    <td className="px-4 py-3">{row.candidates}</td>
                                    <td className="px-4 py-3">{row.applicants}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}

            <HistoricalBrochuresList schoolCode={schoolCode} />
        </div>
    );
}

// 學校的簡章 PDF 依學年度分開存放，不會被隔年上傳的新簡章覆蓋，所以這裡列出的是
// 這間學校目前所有已上架的年度，而不只是這個科系當年度的那一份。下載連結是短效
// 簽章網址，所以按下才即時取，不會預先抓好放著。
function HistoricalBrochuresList({ schoolCode }: { schoolCode: string }) {
    const [documents, setDocuments] = useState<BrochureDocument[] | null>(null);
    const [downloadingYear, setDownloadingYear] = useState<number | null>(null);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        const controller = new AbortController();
        listPublishedBrochures(schoolCode, { signal: controller.signal })
            .then((response) => {
                if (!controller.signal.aborted) setDocuments(response.data);
            })
            .catch(() => {
                if (!controller.signal.aborted) setDocuments([]);
            });
        return () => controller.abort();
    }, [schoolCode]);

    async function download(year: number) {
        setError(null);
        setDownloadingYear(year);
        try {
            const response = await getPublishedBrochureDownload(year, schoolCode);
            window.open(response.url, "_blank", "noopener,noreferrer");
        } catch {
            setError("目前無法取得下載連結，請稍後再試。");
        } finally {
            setDownloadingYear(null);
        }
    }

    if (documents === null || documents.length === 0) return null;

    return (
        <div>
            <h3 className="font-sans text-lg font-medium text-ink">歷史簡章下載</h3>
            <ul className="mt-4 flex flex-wrap gap-2">
                {documents.map((item) => (
                    <li key={item.academic_year}>
                        <button
                            type="button"
                            onClick={() => void download(item.academic_year)}
                            disabled={downloadingYear === item.academic_year}
                            className="inline-flex h-10 shrink-0 cursor-pointer items-center gap-2 rounded-full border border-ink/15 bg-surface px-3 font-sans text-sm font-medium whitespace-nowrap text-ink transition-colors hover:bg-ink/5 focus-visible:ring-2 focus-visible:ring-ink focus-visible:ring-offset-2 focus-visible:outline-none disabled:opacity-60"
                        >
                            <Download aria-hidden className="h-4 w-4 shrink-0" />
                            {item.academic_year} 學年度簡章
                        </button>
                    </li>
                ))}
            </ul>
            {error ? <p className="mt-2 font-sans text-sm text-red-600">{error}</p> : null}
        </div>
    );
}

function Fact({ label, value, compact = false }: BrochureFact & { compact?: boolean }) {
    return (
        <div
            className={
                compact
                    ? "flex items-center gap-3"
                    : "flex flex-col gap-2 sm:flex-row sm:items-start sm:gap-4"
            }
        >
            <dt className="w-fit shrink-0 rounded-[var(--radius-small)] bg-accent-green/55 px-2 py-1 font-sans text-base font-medium text-ink">
                {label}
            </dt>
            <dd className="font-sans text-base leading-7 text-ink/85">{value}</dd>
        </div>
    );
}
