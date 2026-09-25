"use client";

import { useEffect, useState } from "react";
import { Dialog } from "radix-ui";
import { twMerge } from "tailwind-merge";
import { BookOpen, Search, ShieldCheck, X } from "lucide-react";
import Button from "../../components/button";
import {
    forceLogoutUser,
    listUsers,
    reinstateUser,
    setAdmissionsModerator,
    suspendUser,
    type AccountStatus,
    type AdminUser,
    type IdentityStatus
} from "../../lib/api/admin";
import { ApiError } from "../../lib/api/types";
import { useAdmin } from "../admin-context";

const inputClass =
    "rounded-[var(--radius-small)] border border-ink/15 bg-surface px-3 py-2 font-sans text-sm text-ink outline-none focus:border-ink/40";

const identityLabel: Record<IdentityStatus, string> = {
    temporary: "未驗證",
    student: "學生",
    senior: "應屆考生"
};

const statusLabel: Record<AccountStatus, string> = {
    active: "啟用中",
    suspended: "已停權",
    deleted: "已刪除"
};

function describeError(cause: unknown): string {
    if (cause instanceof ApiError) return cause.message || `發生錯誤（${cause.code}）`;
    return "發生未知錯誤，請稍後再試。";
}

export default function UsersView() {
    const { mfaCode } = useAdmin();
    const [status, setStatus] = useState<AccountStatus | "">("");
    const [identity, setIdentity] = useState<IdentityStatus | "">("");
    const [adminOnly, setAdminOnly] = useState(false);
    const [q, setQ] = useState("");

    const [users, setUsers] = useState<AdminUser[] | null>(null);
    const [nextCursor, setNextCursor] = useState("");
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [actionTarget, setActionTarget] = useState<AdminUser | null>(null);

    async function load(cursor?: string) {
        setLoading(true);
        setError(null);
        try {
            const page = await listUsers(
                {
                    status: status || undefined,
                    identity: identity || undefined,
                    role: adminOnly ? "admin" : undefined,
                    q: q.trim() || undefined,
                    cursor
                },
                mfaCode || undefined
            );
            setUsers((prev) => (cursor && prev ? [...prev, ...page.data] : page.data));
            setNextCursor(page.next_cursor);
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setLoading(false);
        }
    }

    useEffect(() => {
        // load() only setStates after its awaited fetch resolves, never
        // synchronously — safe to call directly on mount / when the MFA
        // grant becomes available.
        // eslint-disable-next-line react-hooks/set-state-in-effect
        void load();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [mfaCode]);

    function handleFilterSubmit(event: React.FormEvent) {
        event.preventDefault();
        void load();
    }

    function replaceUser(updated: Partial<AdminUser> & { id: string }) {
        setUsers((prev) =>
            prev ? prev.map((u) => (u.id === updated.id ? { ...u, ...updated } : u)) : prev
        );
    }

    return (
        <div className="flex flex-col gap-6">
            <h1 className="font-serif text-hero-subtitle text-ink">使用者</h1>

            <form onSubmit={handleFilterSubmit} className="flex flex-wrap items-end gap-3">
                <label className="flex flex-col gap-1">
                    <span className="font-sans text-xs text-copy-muted">帳號狀態</span>
                    <select
                        className={inputClass}
                        value={status}
                        onChange={(e) => setStatus(e.target.value as AccountStatus | "")}
                    >
                        <option value="">全部</option>
                        <option value="active">啟用中</option>
                        <option value="suspended">已停權</option>
                        <option value="deleted">已刪除</option>
                    </select>
                </label>
                <label className="flex flex-col gap-1">
                    <span className="font-sans text-xs text-copy-muted">身份</span>
                    <select
                        className={inputClass}
                        value={identity}
                        onChange={(e) => setIdentity(e.target.value as IdentityStatus | "")}
                    >
                        <option value="">全部</option>
                        <option value="temporary">未驗證</option>
                        <option value="student">學生</option>
                        <option value="senior">應屆考生</option>
                    </select>
                </label>
                <label className="flex items-center gap-2 pb-2 font-sans text-sm text-ink">
                    <input
                        type="checkbox"
                        checked={adminOnly}
                        onChange={(e) => setAdminOnly(e.target.checked)}
                    />
                    只看管理員
                </label>
                <label className="flex flex-col gap-1">
                    <span className="font-sans text-xs text-copy-muted">使用者名稱</span>
                    <div className="flex items-center gap-2">
                        <input
                            className={inputClass}
                            placeholder="開頭字串"
                            value={q}
                            onChange={(e) => setQ(e.target.value)}
                        />
                    </div>
                </label>
                <Button type="submit" disabled={loading} className="h-10 gap-2 px-5 text-sm">
                    <Search aria-hidden className="h-4 w-4" />
                    篩選
                </Button>
            </form>

            {error ? <p className="font-sans text-sm text-red-600">{error}</p> : null}

            <div className="overflow-x-auto rounded-[var(--radius-panel)] bg-surface shadow-[var(--shadow-card)]">
                <table className="w-full min-w-[720px] font-sans text-sm">
                    <thead>
                        <tr className="border-b border-ink/10 text-left text-copy-muted">
                            <Th>使用者名稱</Th>
                            <Th>身份</Th>
                            <Th>帳號狀態</Th>
                            <Th>Email</Th>
                            <Th>最後登入</Th>
                            <Th>建立時間</Th>
                            <Th>操作</Th>
                        </tr>
                    </thead>
                    <tbody>
                        {users?.map((user) => (
                            <tr key={user.id} className="border-b border-ink/5 last:border-0">
                                <td className="px-4 py-3 text-ink">
                                    <span className="flex items-center gap-2">
                                        {user.username}
                                        {user.is_admin ? (
                                            <ShieldCheck
                                                aria-hidden
                                                className="h-4 w-4 text-ink/50"
                                            />
                                        ) : user.is_admissions_moderator ? (
                                            <BookOpen
                                                aria-hidden
                                                className="h-4 w-4 text-ink/50"
                                            />
                                        ) : null}
                                    </span>
                                </td>
                                <td className="px-4 py-3 text-copy-muted">
                                    {identityLabel[user.identity_status]}
                                </td>
                                <td className="px-4 py-3">
                                    <StatusBadge status={user.account_status} />
                                </td>
                                <td className="px-4 py-3 text-copy-muted">
                                    {user.email_verified ? "已驗證" : "未驗證"}
                                </td>
                                <td className="px-4 py-3 text-copy-muted">
                                    {formatDate(user.last_login_at)}
                                </td>
                                <td className="px-4 py-3 text-copy-muted">
                                    {formatDate(user.created_at)}
                                </td>
                                <td className="px-4 py-3">
                                    <button
                                        type="button"
                                        onClick={() => setActionTarget(user)}
                                        className="font-sans text-sm text-ink underline underline-offset-2 hover:no-underline"
                                    >
                                        管理
                                    </button>
                                </td>
                            </tr>
                        ))}
                    </tbody>
                </table>
                {users !== null && users.length === 0 ? (
                    <p className="p-6 text-center font-sans text-copy-muted">
                        沒有符合條件的使用者。
                    </p>
                ) : null}
            </div>

            {nextCursor ? (
                <Button
                    onClick={() => void load(nextCursor)}
                    disabled={loading}
                    className="w-fit self-center border border-ink/15 bg-surface px-6 text-ink hover:bg-ink/5"
                >
                    {loading ? "載入中…" : "載入更多"}
                </Button>
            ) : null}

            <UserActionDialog
                key={actionTarget?.id ?? "none"}
                user={actionTarget}
                mfaCode={mfaCode}
                onClose={() => setActionTarget(null)}
                onChanged={replaceUser}
            />
        </div>
    );
}

function Th({ children }: { children: React.ReactNode }) {
    return (
        <th className="px-4 py-3 font-sans text-xs font-bold tracking-wide uppercase">
            {children}
        </th>
    );
}

function StatusBadge({ status }: { status: AccountStatus }) {
    return (
        <span
            className={twMerge(
                "inline-flex rounded-full px-3 py-1 font-sans text-xs font-bold",
                status === "active" && "bg-accent-green-strong text-ink",
                status === "suspended" && "bg-red-100 text-red-700",
                status === "deleted" && "bg-ink/10 text-copy-muted"
            )}
        >
            {statusLabel[status]}
        </span>
    );
}

function UserActionDialog({
    user,
    mfaCode,
    onClose,
    onChanged
}: {
    user: AdminUser | null;
    mfaCode: string;
    onClose: () => void;
    onChanged: (update: Partial<AdminUser> & { id: string }) => void;
}) {
    const [reason, setReason] = useState("");
    const [pending, setPending] = useState<
        "suspend" | "reinstate" | "force-logout" | "grant-admissions" | "revoke-admissions" | null
    >(null);
    const [error, setError] = useState<string | null>(null);
    const [notice, setNotice] = useState<string | null>(null);

    // No reset-on-prop-change effect needed: the parent remounts this
    // component (via `key={actionTarget?.id}`) whenever the dialog target
    // changes, so local state naturally starts fresh.

    async function run(
        action: "suspend" | "reinstate" | "force-logout" | "grant-admissions" | "revoke-admissions"
    ) {
        if (!user) return;
        if (action === "suspend" && reason.trim().length === 0) {
            setError("請填寫停權原因。");
            return;
        }
        setPending(action);
        setError(null);
        try {
            if (action === "suspend") {
                await suspendUser(user.id, reason.trim(), mfaCode || undefined);
                onChanged({
                    id: user.id,
                    account_status: "suspended",
                    suspension_reason: reason.trim()
                });
                setNotice("已停權此帳號。");
            } else if (action === "reinstate") {
                await reinstateUser(user.id, reason.trim(), mfaCode || undefined);
                onChanged({ id: user.id, account_status: "active", suspension_reason: undefined });
                setNotice("已恢復此帳號。");
            } else if (action === "force-logout") {
                const { sessions_revoked } = await forceLogoutUser(
                    user.id,
                    reason.trim(),
                    mfaCode || undefined
                );
                setNotice(`已撤銷 ${sessions_revoked} 個 session。`);
            } else {
                const grant = action === "grant-admissions";
                await setAdmissionsModerator(user.id, grant, reason.trim(), mfaCode || undefined);
                onChanged({ id: user.id, is_admissions_moderator: grant });
                setNotice(grant ? "已授予簡章管理權限。" : "已收回簡章管理權限。");
            }
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setPending(null);
        }
    }

    return (
        <Dialog.Root open={user !== null} onOpenChange={(open) => !open && onClose()}>
            <Dialog.Portal>
                <Dialog.Overlay className="fixed inset-0 z-[80] bg-ink/40" />
                <Dialog.Content className="fixed top-1/2 left-1/2 z-[90] w-[calc(100vw-2.5rem)] max-w-md -translate-x-1/2 -translate-y-1/2 rounded-[var(--radius-panel)] bg-surface p-6 shadow-[var(--shadow-card)]">
                    <div className="flex items-start justify-between gap-4">
                        <div>
                            <Dialog.Title className="font-serif text-xl text-ink">
                                管理 {user?.username}
                            </Dialog.Title>
                            {user ? (
                                <p className="mt-1 font-sans text-sm text-copy-muted">
                                    目前狀態：{statusLabel[user.account_status]}
                                    {user.is_admin
                                        ? "・完整管理員"
                                        : user.is_admissions_moderator
                                          ? "・簡章管理員"
                                          : ""}
                                </p>
                            ) : null}
                        </div>
                        <Dialog.Close asChild>
                            <button
                                type="button"
                                aria-label="關閉"
                                className="rounded-[var(--radius-small)] p-1.5 text-copy-muted hover:bg-ink/5 hover:text-ink"
                            >
                                <X aria-hidden className="h-5 w-5" />
                            </button>
                        </Dialog.Close>
                    </div>

                    <div className="mt-5 flex flex-col gap-3">
                        <label className="flex flex-col gap-1">
                            <span className="font-sans text-xs text-copy-muted">
                                原因（停權必填，恢復 / 強制登出選填）
                            </span>
                            <textarea
                                className="min-h-20 w-full resize-y rounded-[var(--radius-small)] border border-ink/15 bg-surface px-3 py-2 font-sans text-sm text-ink outline-none focus:border-ink/40"
                                maxLength={500}
                                value={reason}
                                onChange={(e) => setReason(e.target.value)}
                            />
                        </label>

                        {error ? <p className="font-sans text-sm text-red-600">{error}</p> : null}
                        {notice ? <p className="font-sans text-sm text-ink/80">{notice}</p> : null}

                        <div className="flex flex-wrap gap-2 pt-2">
                            {user?.account_status === "active" ? (
                                <Button
                                    onClick={() => void run("suspend")}
                                    disabled={pending !== null}
                                    className="bg-red-600 hover:bg-red-700 active:bg-red-800"
                                >
                                    {pending === "suspend" ? "處理中…" : "停權"}
                                </Button>
                            ) : null}
                            {user?.account_status === "suspended" ? (
                                <Button
                                    onClick={() => void run("reinstate")}
                                    disabled={pending !== null}
                                >
                                    {pending === "reinstate" ? "處理中…" : "恢復"}
                                </Button>
                            ) : null}
                            <Button
                                onClick={() => void run("force-logout")}
                                disabled={pending !== null}
                                className="border border-ink/15 bg-surface text-ink hover:bg-ink/5"
                            >
                                {pending === "force-logout" ? "處理中…" : "強制登出所有裝置"}
                            </Button>
                            {!user?.is_admin && user?.is_admissions_moderator ? (
                                <Button
                                    onClick={() => void run("revoke-admissions")}
                                    disabled={pending !== null}
                                    className="border border-ink/15 bg-surface text-ink hover:bg-ink/5"
                                >
                                    {pending === "revoke-admissions" ? "處理中…" : "收回簡章管理權限"}
                                </Button>
                            ) : null}
                            {!user?.is_admin && !user?.is_admissions_moderator ? (
                                <Button
                                    onClick={() => void run("grant-admissions")}
                                    disabled={pending !== null}
                                    className="border border-ink/15 bg-surface text-ink hover:bg-ink/5"
                                >
                                    {pending === "grant-admissions" ? "處理中…" : "授予簡章管理權限"}
                                </Button>
                            ) : null}
                        </div>
                        <p className="font-sans text-xs leading-5 text-copy-muted">
                            簡章管理員只能進入「簡章管理」頁面，看不到其他後台頁面或資料。
                        </p>
                    </div>
                </Dialog.Content>
            </Dialog.Portal>
        </Dialog.Root>
    );
}

function formatDate(iso?: string): string {
    if (!iso) return "—";
    try {
        return new Date(iso).toLocaleString("zh-TW", { dateStyle: "medium", timeStyle: "short" });
    } catch {
        return iso;
    }
}
