"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import {
    AlertTriangle,
    ArrowRight,
    BookOpen,
    CircleHelp,
    FileText,
    MessageCircle,
    ScrollText,
    ShieldCheck,
    Trophy,
    Users
} from "lucide-react";
import {
    getRequireAdminMfa,
    getStats,
    setRequireAdminMfa,
    type AdminStats,
    type OutboxHealth
} from "../lib/api/admin";
import { ApiError } from "../lib/api/types";
import { useAdmin } from "./admin-context";

const panelClass =
    "rounded-[var(--radius-panel)] border border-ink/10 bg-surface/80 p-5 shadow-sm sm:p-6";

const adminModules = [
    {
        href: "/admin/admissions",
        label: "簡章管理",
        description: "同步、上傳與審核官方招生簡章。",
        icon: BookOpen,
        status: "open"
    },
    {
        href: "/admin/users",
        label: "使用者管理",
        description: "查看帳號狀態、身份與管理員權限。",
        icon: Users,
        status: "open"
    },
    {
        href: "/admin/audit-log",
        label: "稽核紀錄",
        description: "追蹤管理操作與資料變更歷程。",
        icon: ScrollText,
        status: "open"
    },
    {
        label: "申請管理",
        description: "查看與處理學生的特殊選才申請。",
        icon: FileText,
        status: "planned"
    },
    {
        label: "文章管理",
        description: "管理心得文章的審核與發布狀態。",
        icon: FileText,
        status: "planned"
    },
    {
        label: "論壇與聊天室",
        description: "管理討論內容、社群空間與訊息。",
        icon: MessageCircle,
        status: "planned"
    },
    {
        label: "身份驗證",
        description: "審核學生與應屆考生的身份申請。",
        icon: ShieldCheck,
        status: "planned"
    },
    {
        label: "放榜批次",
        description: "管理放榜資料的審核與發布。",
        icon: Trophy,
        status: "planned"
    },
    {
        label: "客服單",
        description: "追蹤使用者回報與客服處理進度。",
        icon: CircleHelp,
        status: "planned"
    }
] as const;

export default function AdminDashboardPage() {
    const { mfaCode } = useAdmin();
    const [stats, setStats] = useState<AdminStats | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [requireMfa, setRequireMfaState] = useState<boolean | null>(null);
    const [mfaToggleBusy, setMfaToggleBusy] = useState(false);
    const [mfaToggleError, setMfaToggleError] = useState<string | null>(null);

    useEffect(() => {
        let ignore = false;
        getStats(mfaCode || undefined)
            .then((data) => !ignore && setStats(data))
            .catch(
                (cause) =>
                    !ignore && setError(cause instanceof ApiError ? cause.message : "載入失敗")
            );
        getRequireAdminMfa(mfaCode || undefined)
            .then((data) => !ignore && setRequireMfaState(data.value))
            .catch(() => {
                // Leave requireMfa as null (toggle just stays hidden) —
                // this section is a nice-to-have, not core to the page.
            });
        return () => {
            ignore = true;
        };
    }, [mfaCode]);

    async function handleToggleRequireMfa() {
        if (requireMfa === null || mfaToggleBusy) return;
        setMfaToggleBusy(true);
        setMfaToggleError(null);
        const next = !requireMfa;
        try {
            const result = await setRequireAdminMfa(next, mfaCode || undefined);
            setRequireMfaState(result.value);
        } catch (cause) {
            setMfaToggleError(cause instanceof ApiError ? cause.message : "更新失敗，請稍後再試。");
        } finally {
            setMfaToggleBusy(false);
        }
    }

    return (
        <div className="article-dots -mx-5 -my-8 flex min-h-full flex-col border-y border-ink/5 px-5 py-10 sm:-mx-6 sm:-my-8 sm:px-6 lg:-mx-16 lg:-my-10 lg:px-16 lg:py-12">
            <div className="mx-auto flex w-full max-w-screen-xl flex-col gap-10">
                <div className="flex flex-wrap items-end justify-between gap-4 border-b border-ink/10 pb-8">
                    <div>
                        <p className="font-sans text-sm text-ink/60">S.T.A 管理後台</p>
                        <h1 className="mt-2 font-serif text-4xl tracking-[-0.04em] text-ink sm:text-5xl lg:text-6xl">
                            管理總覽
                        </h1>
                        <p className="mt-3 max-w-2xl font-sans text-sm leading-6 text-ink/60 sm:text-base">
                            從這裡進入各個管理模組，掌握平台資料與目前的系統狀態。
                        </p>
                    </div>
                    {stats ? (
                        <p className="font-sans text-xs text-copy-muted">
                            資料截至 {formatDateTime(stats.generated_at)}
                        </p>
                    ) : null}
                </div>

                <section className="flex flex-col gap-4">
                    <div>
                        <h2 className="font-serif text-2xl text-ink">管理模組</h2>
                        <p className="mt-1 font-sans text-sm text-copy-muted">
                            已開放的功能可以直接進入；其他模組會依照後端功能完成狀態陸續開放。
                        </p>
                    </div>
                    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
                        {adminModules.map((module) => {
                            const Icon = module.icon;
                            const content = (
                                <>
                                    <div className="flex items-start justify-between gap-4">
                                        <span
                                            className={
                                                module.status === "open"
                                                    ? "flex h-10 w-10 items-center justify-center rounded-full bg-accent-yellow/70 text-ink"
                                                    : "flex h-10 w-10 items-center justify-center rounded-full bg-ink/5 text-ink/50"
                                            }
                                        >
                                            <Icon aria-hidden className="h-5 w-5" />
                                        </span>
                                        <span
                                            className={
                                                module.status === "open"
                                                    ? "rounded-full bg-accent-green/70 px-3 py-1 font-sans text-xs font-bold text-ink"
                                                    : "rounded-full bg-ink/5 px-3 py-1 font-sans text-xs text-copy-muted"
                                            }
                                        >
                                            {module.status === "open" ? "已開放" : "規劃中"}
                                        </span>
                                    </div>
                                    <div className="mt-6 flex items-end justify-between gap-3">
                                        <div>
                                            <h3 className="font-serif text-xl text-ink">
                                                {module.label}
                                            </h3>
                                            <p className="mt-2 font-sans text-sm leading-6 text-copy-muted">
                                                {module.description}
                                            </p>
                                        </div>
                                        {module.status === "open" ? (
                                            <ArrowRight
                                                aria-hidden
                                                className="mb-1 h-5 w-5 shrink-0 text-ink/50 transition-transform group-hover:translate-x-1"
                                            />
                                        ) : null}
                                    </div>
                                </>
                            );

                            return "href" in module ? (
                                <Link
                                    key={module.label}
                                    href={module.href}
                                    className="group min-h-48 rounded-[var(--radius-panel)] border border-ink/10 bg-surface/80 p-5 shadow-sm transition hover:-translate-y-0.5 hover:border-ink/20 hover:shadow-[var(--shadow-card)] sm:p-6"
                                >
                                    {content}
                                </Link>
                            ) : (
                                <div
                                    key={module.label}
                                    className="min-h-48 rounded-[var(--radius-panel)] border border-ink/5 bg-surface/50 p-5 opacity-75 sm:p-6"
                                >
                                    {content}
                                </div>
                            );
                        })}
                    </div>
                </section>

                <section className={panelClass}>
                    <div className="flex flex-wrap items-start justify-between gap-4">
                        <div>
                            <h2 className="font-serif text-xl text-ink">系統設定</h2>
                            <p className="mt-2 max-w-xl font-sans text-sm leading-6 text-copy-muted">
                                管理員雙重驗證（MFA）。目前前台還沒有綁定驗證器的畫面，開啟後會讓
                                所有 admin 帳號卡在無法通過的驗證步驟——建議在那個流程做好之前保持關閉。
                            </p>
                        </div>
                        <button
                            type="button"
                            onClick={() => void handleToggleRequireMfa()}
                            disabled={requireMfa === null || mfaToggleBusy}
                            className={
                                requireMfa
                                    ? "shrink-0 rounded-full bg-accent-green-strong px-4 py-2 font-sans text-sm font-bold text-ink transition-opacity disabled:opacity-50"
                                    : "shrink-0 rounded-full border border-ink/15 bg-surface px-4 py-2 font-sans text-sm text-ink/70 transition-opacity hover:text-ink disabled:opacity-50"
                            }
                        >
                            {requireMfa === null
                                ? "載入中…"
                                : mfaToggleBusy
                                  ? "更新中…"
                                  : requireMfa
                                    ? "已啟用（點擊關閉）"
                                    : "已關閉（點擊啟用）"}
                        </button>
                    </div>
                    {mfaToggleError ? (
                        <p className="mt-3 font-sans text-sm text-red-600">{mfaToggleError}</p>
                    ) : null}
                </section>

                {error ? <p className="font-sans text-sm text-red-600">{error}</p> : null}

                {!stats ? (
                    <p className="font-sans text-copy-muted">載入系統概況中…</p>
                ) : (
                    <>
                        <section className="flex flex-col gap-4">
                            <div>
                                <h2 className="font-serif text-2xl text-ink">系統概況</h2>
                                <p className="mt-1 font-sans text-sm text-copy-muted">
                                    各服務目前的資料數量與處理狀態。
                                </p>
                            </div>
                            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
                                <StatGroup
                                    title="帳號"
                                    entries={[
                                        ["總數", stats.accounts.total],
                                        ["啟用中", stats.accounts.active],
                                        ["已停權", stats.accounts.suspended],
                                        ["已驗證身份", stats.accounts.verified],
                                        ["學生", stats.accounts.students],
                                        ["應屆考生", stats.accounts.seniors]
                                    ]}
                                />
                                <StatGroup
                                    title="申請"
                                    entries={[
                                        ["總數", stats.applications.total],
                                        ["草稿", stats.applications.draft],
                                        ["已確認", stats.applications.confirmed],
                                        ["已撤回", stats.applications.withdrawn],
                                        ["已封存", stats.applications.archived]
                                    ]}
                                />
                                <StatGroup
                                    title="心得文章"
                                    entries={[
                                        ["總數", stats.experiences.total],
                                        ["已發布", stats.experiences.published],
                                        ["隱藏", stats.experiences.hidden],
                                        ["已下架", stats.experiences.unpublished]
                                    ]}
                                />
                                <StatGroup
                                    title="論壇"
                                    entries={[
                                        ["空間", stats.forum.spaces],
                                        ["討論串", stats.forum.threads],
                                        ["貼文", stats.forum.posts]
                                    ]}
                                />
                                <StatGroup
                                    title="聊天室"
                                    entries={[["閒聊訊息", stats.chat.lounge_messages]]}
                                />
                                <StatGroup
                                    title="客服單"
                                    entries={[
                                        ["總數", stats.support_tickets.total],
                                        ["處理中", stats.support_tickets.open],
                                        ["已結案", stats.support_tickets.closed]
                                    ]}
                                />
                                <StatGroup
                                    title="身份驗證申請"
                                    entries={[
                                        ["待審核", stats.verification_requests.pending],
                                        ["已通過", stats.verification_requests.approved],
                                        ["已拒絕", stats.verification_requests.rejected]
                                    ]}
                                />
                                <StatGroup
                                    title="放榜批次"
                                    entries={[
                                        ["總數", stats.result_batches.total],
                                        ["待審核", stats.result_batches.pending_review],
                                        ["已發布", stats.result_batches.published]
                                    ]}
                                />
                                <StatGroup
                                    title="稽核紀錄"
                                    entries={[["總筆數", stats.audit_log.total]]}
                                />
                            </div>
                        </section>

                        <section className="flex flex-col gap-4">
                            <h2 className="font-serif text-xl text-ink">背景佇列健康度</h2>
                            <div className={panelClass}>
                                <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
                                    <OutboxCard label="Email" health={stats.outbox.email} />
                                    <OutboxCard
                                        label="聊天室同步"
                                        health={stats.outbox.chat_sync}
                                    />
                                    <OutboxCard
                                        label="客服 Discord"
                                        health={stats.outbox.support_discord}
                                    />
                                    <OutboxCard
                                        label="意願通知"
                                        health={stats.outbox.willingness_notifications}
                                    />
                                </div>
                            </div>
                        </section>
                    </>
                )}
            </div>
        </div>
    );
}

function StatGroup({ title, entries }: { title: string; entries: Array<[string, number]> }) {
    return (
        <div className={panelClass}>
            <p className="font-serif text-lg text-ink">{title}</p>
            <dl className="mt-4 flex flex-col gap-2">
                {entries.map(([label, value]) => (
                    <div key={label} className="flex items-baseline justify-between gap-3">
                        <dt className="font-sans text-sm text-copy-muted">{label}</dt>
                        <dd className="font-sans text-lg font-bold text-ink">
                            {value.toLocaleString("zh-TW")}
                        </dd>
                    </div>
                ))}
            </dl>
        </div>
    );
}

function OutboxCard({ label, health }: { label: string; health: OutboxHealth }) {
    const alert = health.abandoned > 0;
    return (
        <div
            className={
                alert
                    ? "rounded-[var(--radius-small)] border border-red-200 bg-red-50 p-4"
                    : "rounded-[var(--radius-small)] border border-ink/10 p-4"
            }
        >
            <div className="flex items-center gap-2">
                <p className="font-sans text-sm font-bold text-ink">{label}</p>
                {alert ? <AlertTriangle aria-hidden className="h-4 w-4 text-red-600" /> : null}
            </div>
            <div className="mt-2 flex flex-col gap-1 font-sans text-sm text-copy-muted">
                <span>待處理 {health.pending}</span>
                <span>失敗重試中 {health.failed}</span>
                <span className={alert ? "font-bold text-red-600" : undefined}>
                    放棄 {health.abandoned}
                </span>
            </div>
        </div>
    );
}

function formatDateTime(iso: string): string {
    try {
        return new Date(iso).toLocaleString("zh-TW", { dateStyle: "medium", timeStyle: "short" });
    } catch {
        return iso;
    }
}
