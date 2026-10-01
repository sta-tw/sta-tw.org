"use client";

import { useEffect, useState } from "react";
import { twMerge } from "tailwind-merge";
import { BookOpen, Search, ShieldCheck } from "lucide-react";
import Button from "../../components/button";
import {
    createUser,
    forceLogoutUser,
    getUser,
    reinstateUser,
    resendActivation,
    resendEmailVerification,
    resetUserPassword,
    setAdmissionsModerator,
    suspendUser,
    updateUser,
    listUsers,
    type AccountStatus,
    type AdminUser,
    type AdminUserDetail,
    type IdentityStatus
} from "../../lib/api/admin";
import { useAdmin } from "../admin-context";
import {
    AdminDialog,
    AdminTable,
    Badge,
    describeError,
    EmptyState,
    ErrorText,
    formatDate,
    inputClass,
    LoadingState,
    Td,
    textareaClass,
    Th
} from "../admin-ui";

const identityLabel: Record<IdentityStatus, string> = {
    temporary: "未驗證",
    student: "學生",
    senior: "應屆考生"
};

function describeIdentity(
    user: Pick<AdminUser, "identity_status" | "is_admissions_moderator" | "is_admin">
): string {
    const labels = [identityLabel[user.identity_status]];
    if (user.is_admissions_moderator) labels.push("版主");
    if (user.is_admin) labels.push("管理員");
    return labels.join("／");
}

const statusLabel: Record<AccountStatus, string> = {
    active: "啟用中",
    pending_verification: "未啟用",
    suspended: "已停權",
    deleted: "已刪除"
};

const statusTone: Record<AccountStatus, "positive" | "warning" | "negative" | "neutral"> = {
    active: "positive",
    pending_verification: "warning",
    suspended: "negative",
    deleted: "neutral"
};

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
    const [creating, setCreating] = useState(false);

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

    function prependUser(created: AdminUser) {
        setUsers((prev) => (prev ? [created, ...prev] : [created]));
    }

    return (
        <div className="flex flex-col gap-6">
            <div className="flex flex-wrap items-end justify-between gap-4">
                <h1 className="font-serif text-hero-subtitle text-ink">使用者</h1>
                <Button
                    type="button"
                    onClick={() => setCreating(true)}
                    className="h-10 gap-2 px-5 text-sm"
                >
                    + 建立帳號
                </Button>
            </div>

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
                        <option value="pending_verification">未啟用</option>
                        <option value="suspended">已停權</option>
                        <option value="deleted">已刪除</option>
                    </select>
                </label>
                <label className="flex flex-col gap-1">
                    <span className="font-sans text-xs text-copy-muted">驗證身份</span>
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

            {error ? <ErrorText>{error}</ErrorText> : null}

            <div className="overflow-x-auto rounded-[var(--radius-panel)] border border-ink/10 bg-surface shadow-[var(--shadow-card)]">
                <table className="w-full min-w-[720px] font-sans text-sm">
                    <thead>
                        <tr className="border-b border-ink/10 text-left text-copy-muted">
                            <Th>使用者名稱</Th>
                            <Th>身份／角色</Th>
                            <Th>帳號狀態</Th>
                            <Th>Email</Th>
                            <Th>最後登入</Th>
                            <Th>建立時間</Th>
                            <Th>操作</Th>
                        </tr>
                    </thead>
                    <tbody className="[&>tr]:border-b [&>tr]:border-ink/5 [&>tr:last-child]:border-0">
                        {users?.map((user) => (
                            <tr key={user.id}>
                                <Td>
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
                                </Td>
                                <Td className="text-copy-muted">{describeIdentity(user)}</Td>
                                <Td>
                                    <Badge tone={statusTone[user.account_status]}>
                                        {statusLabel[user.account_status]}
                                    </Badge>
                                </Td>
                                <Td className="text-copy-muted">
                                    {user.email_verified ? "已驗證" : "未驗證"}
                                </Td>
                                <Td className="text-copy-muted">{formatDate(user.last_login_at)}</Td>
                                <Td className="text-copy-muted">{formatDate(user.created_at)}</Td>
                                <Td>
                                    <button
                                        type="button"
                                        onClick={() => setActionTarget(user)}
                                        className="font-sans text-sm text-ink underline underline-offset-2 hover:no-underline"
                                    >
                                        管理
                                    </button>
                                </Td>
                            </tr>
                        ))}
                    </tbody>
                </table>
                {users !== null && users.length === 0 ? <EmptyState label="沒有符合條件的使用者。" /> : null}
            </div>

            {nextCursor ? (
                <Button
                    variant="secondary"
                    onClick={() => void load(nextCursor)}
                    disabled={loading}
                    className="w-fit self-center px-6"
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
            <CreateUserDialog
                open={creating}
                mfaCode={mfaCode}
                onClose={() => setCreating(false)}
                onCreated={prependUser}
            />
        </div>
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
        | "suspend"
        | "reinstate"
        | "force-logout"
        | "grant-admissions"
        | "revoke-admissions"
        | "reset-password"
        | "resend-activation"
        | "resend-verification"
        | null
    >(null);
    const [error, setError] = useState<string | null>(null);
    const [notice, setNotice] = useState<string | null>(null);

    const [detail, setDetail] = useState<AdminUserDetail | null>(null);
    const [detailError, setDetailError] = useState<string | null>(null);
    const [editUsername, setEditUsername] = useState("");
    const [editIdentity, setEditIdentity] = useState<IdentityStatus>("temporary");
    const [editEmail, setEditEmail] = useState("");
    const [editSchoolEmail, setEditSchoolEmail] = useState("");
    const [editSaving, setEditSaving] = useState(false);
    const [editError, setEditError] = useState<string | null>(null);

    // No reset-on-prop-change effect needed: the parent remounts this
    // component (via `key={actionTarget?.id}`) whenever the dialog target
    // changes, so local state naturally starts fresh.
    useEffect(() => {
        if (!user) return;
        let cancelled = false;
        void (async () => {
            try {
                const full = await getUser(user.id, mfaCode || undefined);
                if (cancelled) return;
                setDetail(full);
                setEditUsername(full.username);
                setEditIdentity(full.identity_status);
                setEditEmail(full.email);
                setEditSchoolEmail(full.school_email);
            } catch (cause) {
                if (!cancelled) setDetailError(describeError(cause));
            }
        })();
        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [user?.id]);

    async function saveEdits() {
        if (!user) return;
        setEditSaving(true);
        setEditError(null);
        try {
            const input: Parameters<typeof updateUser>[1] = {};
            if (detail && editUsername.trim() !== detail.username) input.username = editUsername.trim();
            if (detail && editIdentity !== detail.identity_status) input.identity_status = editIdentity;
            if (detail && editEmail.trim() !== detail.email) input.email = editEmail.trim();
            if (detail && editSchoolEmail.trim() !== detail.school_email)
                input.school_email = editSchoolEmail.trim();
            if (Object.keys(input).length === 0) {
                setEditSaving(false);
                return;
            }
            await updateUser(user.id, input, mfaCode || undefined);
            const refreshed = await getUser(user.id, mfaCode || undefined);
            setDetail(refreshed);
            onChanged({
                id: user.id,
                username: refreshed.username,
                identity_status: refreshed.identity_status
            });
            setNotice("已儲存變更。");
        } catch (cause) {
            setEditError(describeError(cause));
        } finally {
            setEditSaving(false);
        }
    }

    async function run(
        action:
            | "suspend"
            | "reinstate"
            | "force-logout"
            | "grant-admissions"
            | "revoke-admissions"
            | "reset-password"
            | "resend-activation"
            | "resend-verification"
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
            } else if (action === "reset-password") {
                await resetUserPassword(user.id, mfaCode || undefined);
                setNotice("已寄出密碼重置信。");
            } else if (action === "resend-activation") {
                await resendActivation(user.id, mfaCode || undefined);
                setNotice("已重寄啟用信到目前登記的學校信箱。");
            } else if (action === "resend-verification") {
                await resendEmailVerification(user.id, mfaCode || undefined);
                setNotice("已重寄驗證信。");
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
        <AdminDialog
            open={user !== null}
            onOpenChange={(open) => !open && onClose()}
            title={`管理 ${user?.username ?? ""}`}
            description={
                user ? (
                    <>
                        目前狀態：{statusLabel[user.account_status]}・身份／角色：{describeIdentity(user)}
                    </>
                ) : undefined
            }
            maxWidth="max-w-md"
        >
            <div className="flex flex-col gap-3 border-b border-ink/10 pb-5">
                <p className="font-sans text-xs font-bold text-copy-muted">帳號資料</p>
                {detailError ? (
                    <ErrorText>{detailError}</ErrorText>
                ) : !detail ? (
                    <LoadingState />
                ) : (
                    <>
                        <label className="flex flex-col gap-1">
                            <span className="font-sans text-xs text-copy-muted">使用者名稱</span>
                            <input
                                className={inputClass}
                                value={editUsername}
                                onChange={(e) => setEditUsername(e.target.value)}
                            />
                        </label>
                        <label className="flex flex-col gap-1">
                            <span className="font-sans text-xs text-copy-muted">身份狀態</span>
                            <select
                                className={inputClass}
                                value={editIdentity}
                                onChange={(e) => setEditIdentity(e.target.value as IdentityStatus)}
                            >
                                <option value="temporary">未驗證</option>
                                <option value="student">學生</option>
                                <option value="senior">應屆考生</option>
                            </select>
                        </label>
                        <label className="flex flex-col gap-1">
                            <span className="font-sans text-xs text-copy-muted">
                                常用 Email（登入通知 / 密碼重置信收件地址）
                            </span>
                            <input
                                type="email"
                                className={inputClass}
                                value={editEmail}
                                onChange={(e) => setEditEmail(e.target.value)}
                            />
                        </label>
                        <label className="flex flex-col gap-1">
                            <span className="font-sans text-xs text-copy-muted">
                                學校信箱（*.edu.tw，未啟用帳號的啟用信會寄到這裡）
                            </span>
                            <input
                                type="email"
                                className={inputClass}
                                placeholder={detail.school_email ? undefined : "此帳號沒有登記學校信箱"}
                                value={editSchoolEmail}
                                onChange={(e) => setEditSchoolEmail(e.target.value)}
                            />
                        </label>
                        {editError ? <ErrorText>{editError}</ErrorText> : null}
                        <Button
                            type="button"
                            variant="secondary"
                            onClick={() => void saveEdits()}
                            disabled={editSaving}
                            className="h-10 w-fit px-4 text-sm"
                        >
                            {editSaving ? "儲存中…" : "儲存資料"}
                        </Button>
                        {detail.account_status === "pending_verification" ? (
                            <Button
                                type="button"
                                variant="secondary"
                                onClick={() => void run("resend-activation")}
                                disabled={pending !== null}
                                className="h-10 w-fit px-4 text-sm"
                            >
                                {pending === "resend-activation" ? "處理中…" : "重寄啟用信到學校信箱"}
                            </Button>
                        ) : !detail.email_verified ? (
                            <Button
                                type="button"
                                variant="secondary"
                                onClick={() => void run("resend-verification")}
                                disabled={pending !== null}
                                className="h-10 w-fit px-4 text-sm"
                            >
                                {pending === "resend-verification" ? "處理中…" : "重寄 Email 驗證信"}
                            </Button>
                        ) : null}
                    </>
                )}
            </div>

            <div className="flex flex-col gap-3">
                <label className="flex flex-col gap-1">
                    <span className="font-sans text-xs text-copy-muted">
                        原因（停權必填，恢復 / 強制登出選填）
                    </span>
                    <textarea
                        className={textareaClass}
                        maxLength={500}
                        value={reason}
                        onChange={(e) => setReason(e.target.value)}
                    />
                </label>

                {error ? <ErrorText>{error}</ErrorText> : null}
                {notice ? <p className="font-sans text-sm text-ink/80">{notice}</p> : null}

                <div className="flex flex-wrap gap-2 pt-2">
                    {user?.account_status === "active" ? (
                        <Button
                            variant="danger"
                            onClick={() => void run("suspend")}
                            disabled={pending !== null}
                            className="h-10 px-4 text-sm"
                        >
                            {pending === "suspend" ? "處理中…" : "停權"}
                        </Button>
                    ) : null}
                    {user?.account_status === "suspended" ? (
                        <Button
                            onClick={() => void run("reinstate")}
                            disabled={pending !== null}
                            className="h-10 px-4 font-sans text-sm"
                        >
                            {pending === "reinstate" ? "處理中…" : "恢復"}
                        </Button>
                    ) : null}
                    <Button
                        variant="secondary"
                        onClick={() => void run("force-logout")}
                        disabled={pending !== null}
                        className="h-10 px-4 text-sm"
                    >
                        {pending === "force-logout" ? "處理中…" : "強制登出所有裝置"}
                    </Button>
                    {user?.account_status === "active" ? (
                        <Button
                            variant="secondary"
                            onClick={() => void run("reset-password")}
                            disabled={pending !== null}
                            className="h-10 px-4 text-sm"
                        >
                            {pending === "reset-password" ? "處理中…" : "重置密碼"}
                        </Button>
                    ) : null}
                    {!user?.is_admin && user?.is_admissions_moderator ? (
                        <Button
                            variant="secondary"
                            onClick={() => void run("revoke-admissions")}
                            disabled={pending !== null}
                            className="h-10 px-4 text-sm"
                        >
                            {pending === "revoke-admissions" ? "處理中…" : "收回簡章管理權限"}
                        </Button>
                    ) : null}
                    {!user?.is_admin && !user?.is_admissions_moderator ? (
                        <Button
                            variant="secondary"
                            onClick={() => void run("grant-admissions")}
                            disabled={pending !== null}
                            className="h-10 px-4 text-sm"
                        >
                            {pending === "grant-admissions" ? "處理中…" : "授予簡章管理權限"}
                        </Button>
                    ) : null}
                </div>
                <p className="font-sans text-xs leading-5 text-copy-muted">
                    版主只能進入「簡章管理」頁面，看不到其他後台頁面或資料。
                </p>
            </div>
        </AdminDialog>
    );
}

function CreateUserDialog({
    open,
    mfaCode,
    onClose,
    onCreated
}: {
    open: boolean;
    mfaCode: string;
    onClose: () => void;
    onCreated: (created: AdminUser) => void;
}) {
    const [mode, setMode] = useState<"invite" | "service" | "password">("invite");
    const [username, setUsername] = useState("");
    const [email, setEmail] = useState("");
    const [label, setLabel] = useState("");
    const [grantAdmin, setGrantAdmin] = useState(false);
    const [password, setPassword] = useState("");
    const [grantAdmissionsModerator, setGrantAdmissionsModerator] = useState(false);
    const [pending, setPending] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [issuedToken, setIssuedToken] = useState<string | null>(null);

    function reset() {
        setMode("invite");
        setUsername("");
        setEmail("");
        setLabel("");
        setGrantAdmin(false);
        setPassword("");
        setGrantAdmissionsModerator(false);
        setError(null);
        setIssuedToken(null);
    }

    async function submit() {
        setError(null);
        if (username.trim().length < 3) {
            setError("使用者名稱至少需要 3 個字元。");
            return;
        }
        setPending(true);
        try {
            if (mode === "invite") {
                if (!email.trim()) {
                    setError("請填寫 Email。");
                    setPending(false);
                    return;
                }
                const created = await createUser(
                    { mode: "invite", username: username.trim(), email: email.trim() },
                    mfaCode || undefined
                );
                onCreated(created.account);
                reset();
                onClose();
            } else if (mode === "service") {
                const created = await createUser(
                    {
                        mode: "service",
                        username: username.trim(),
                        label: label.trim(),
                        grant_admin: grantAdmin
                    },
                    mfaCode || undefined
                );
                onCreated(created.account);
                setIssuedToken((created as { token: string }).token);
            } else {
                if (password.length < 8) {
                    setError("密碼至少需要 8 個字元。");
                    setPending(false);
                    return;
                }
                const created = await createUser(
                    {
                        mode: "password",
                        username: username.trim(),
                        password,
                        grant_admissions_moderator: grantAdmissionsModerator
                    },
                    mfaCode || undefined
                );
                onCreated(created.account);
                reset();
                onClose();
            }
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setPending(false);
        }
    }

    return (
        <AdminDialog
            open={open}
            onOpenChange={(next) => {
                if (!next) {
                    reset();
                    onClose();
                }
            }}
            title="建立帳號"
            maxWidth="max-w-md"
        >
            {issuedToken ? (
                <>
                    <p className="font-sans text-sm text-ink">
                        機器帳號已建立。這組 token 只會顯示這一次，請立刻複製保存：
                    </p>
                    <code className="block overflow-x-auto rounded-[var(--radius-small)] border border-ink/15 bg-ink/[0.03] px-3 py-2 font-mono text-xs text-ink">
                        {issuedToken}
                    </code>
                    <Button
                        type="button"
                        onClick={() => {
                            reset();
                            onClose();
                        }}
                        className="mt-2 h-10 px-4 font-sans text-sm"
                    >
                        完成
                    </Button>
                </>
            ) : (
                <>
                    <div className="flex gap-2">
                        <button
                            type="button"
                            onClick={() => setMode("invite")}
                            className={twMerge(
                                "flex-1 rounded-[var(--radius-small)] px-3 py-2 font-sans text-sm font-bold",
                                mode === "invite"
                                    ? "bg-ink text-surface"
                                    : "bg-ink/10 text-copy-muted hover:bg-ink/15"
                            )}
                        >
                            邀請使用者
                        </button>
                        <button
                            type="button"
                            onClick={() => setMode("service")}
                            className={twMerge(
                                "flex-1 rounded-[var(--radius-small)] px-3 py-2 font-sans text-sm font-bold",
                                mode === "service"
                                    ? "bg-ink text-surface"
                                    : "bg-ink/10 text-copy-muted hover:bg-ink/15"
                            )}
                        >
                            機器帳號
                        </button>
                        <button
                            type="button"
                            onClick={() => setMode("password")}
                            className={twMerge(
                                "flex-1 rounded-[var(--radius-small)] px-3 py-2 font-sans text-sm font-bold",
                                mode === "password"
                                    ? "bg-ink text-surface"
                                    : "bg-ink/10 text-copy-muted hover:bg-ink/15"
                            )}
                        >
                            指定密碼
                        </button>
                    </div>

                    <label className="flex flex-col gap-1">
                        <span className="font-sans text-xs text-copy-muted">使用者名稱</span>
                        <input
                            className={inputClass}
                            value={username}
                            onChange={(e) => setUsername(e.target.value)}
                        />
                    </label>

                    {mode === "invite" ? (
                        <label className="flex flex-col gap-1">
                            <span className="font-sans text-xs text-copy-muted">Email</span>
                            <input
                                type="email"
                                className={inputClass}
                                value={email}
                                onChange={(e) => setEmail(e.target.value)}
                            />
                            <span className="mt-1 font-sans text-xs leading-5 text-copy-muted">
                                會直接建立已啟用帳號，並寄一封設定密碼的連結信到這個地址。
                            </span>
                        </label>
                    ) : mode === "service" ? (
                        <>
                            <label className="flex flex-col gap-1">
                                <span className="font-sans text-xs text-copy-muted">用途標籤</span>
                                <input
                                    className={inputClass}
                                    placeholder="例如：大表匯入"
                                    value={label}
                                    onChange={(e) => setLabel(e.target.value)}
                                />
                            </label>
                            <label className="flex items-center gap-2 font-sans text-sm text-ink">
                                <input
                                    type="checkbox"
                                    checked={grantAdmin}
                                    onChange={(e) => setGrantAdmin(e.target.checked)}
                                />
                                同時授予管理員權限（可呼叫後台 API）
                            </label>
                            <span className="font-sans text-xs leading-5 text-copy-muted">
                                會發一組 API token，只顯示這一次，之後無法再次查看。
                            </span>
                        </>
                    ) : (
                        <>
                            <label className="flex flex-col gap-1">
                                <span className="font-sans text-xs text-copy-muted">
                                    密碼（至少 8 個字元）
                                </span>
                                <input
                                    type="text"
                                    className={`${inputClass} font-mono`}
                                    value={password}
                                    onChange={(e) => setPassword(e.target.value)}
                                />
                            </label>
                            <label className="flex items-center gap-2 font-sans text-sm text-ink">
                                <input
                                    type="checkbox"
                                    checked={grantAdmissionsModerator}
                                    onChange={(e) => setGrantAdmissionsModerator(e.target.checked)}
                                />
                                同時授予簡章管理權限
                            </label>
                            <span className="font-sans text-xs leading-5 text-copy-muted">
                                不需要 email，直接用這組帳號密碼登入；沒有寄信、沒有一次性連結。
                            </span>
                        </>
                    )}

                    {error ? <ErrorText>{error}</ErrorText> : null}

                    <Button
                        type="button"
                        onClick={() => void submit()}
                        disabled={pending}
                        className="mt-2 h-10 px-4 font-sans text-sm"
                    >
                        {pending ? "建立中…" : "建立"}
                    </Button>
                </>
            )}
        </AdminDialog>
    );
}
