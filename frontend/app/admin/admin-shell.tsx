"use client";

import { useEffect, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { twMerge } from "tailwind-merge";
import { ArrowLeft, Loader2, Lock, LogOut, ShieldAlert } from "lucide-react";
import Button from "../components/button";
import { getCurrentAccount, logout, verifyAdminMfa } from "../lib/api/auth";
import { getStats } from "../lib/api/admin";
import { listAdminAdmissionPrograms } from "../lib/api/admissions";
import { ApiError, type Account } from "../lib/api/types";
import { publicPath } from "../lib/public-path";
import { AdminContextProvider } from "./admin-context";

type Scope = "full" | "admissions";

type Status =
    | { name: "checking" }
    | { name: "signed-out" }
    | { name: "forbidden" }
    | { name: "mfa-required" }
    | { name: "rate-limited" }
    | { name: "error"; message: string }
    | { name: "ready"; account: Account; scope: Scope };

const ADMISSIONS_ONLY_PATH = "/admin/admissions";

const fullNavItems = [
    { href: "/admin", label: "總覽" },
    { href: "/admin/admissions", label: "簡章管理" },
    { href: "/admin/users", label: "使用者" },
    { href: "/admin/audit-log", label: "稽核紀錄" }
];

const admissionsOnlyNavItems = [{ href: ADMISSIONS_ONLY_PATH, label: "簡章管理" }];

const logoIcon = publicPath("/logo.svg");

export default function AdminShell({ children }: { children: React.ReactNode }) {
    const pathname = usePathname();
    const router = useRouter();
    const [status, setStatus] = useState<Status>({ name: "checking" });
    const [mfaCode, setMfaCode] = useState("");
    const [mfaInput, setMfaInput] = useState("");
    const [mfaSubmitting, setMfaSubmitting] = useState(false);
    const [mfaError, setMfaError] = useState<string | null>(null);

    async function probe() {
        setStatus({ name: "checking" });
        let account: Account;
        let isAdmin = false;
        let canManageAdmissions = false;
        try {
            const me = await getCurrentAccount();
            account = me.account;
            isAdmin = me.is_admin;
            canManageAdmissions = me.can_manage_admissions;
        } catch {
            setStatus({ name: "signed-out" });
            return;
        }
        if (!isAdmin && !canManageAdmissions) {
            setStatus({ name: "forbidden" });
            return;
        }
        const scope: Scope = isAdmin ? "full" : "admissions";
        // An admissions-only moderator has no access to anything else under
        // /admin — including the /admin/stats probe below, which would just
        // 403. Redirect them straight to the one page they can use instead
        // of surfacing that as an error.
        if (scope === "admissions" && !pathname.startsWith(ADMISSIONS_ONLY_PATH)) {
            router.replace(ADMISSIONS_ONLY_PATH);
            return;
        }
        try {
            // The cheapest call scoped to this account's actual permissions:
            // it's how we learn whether the role check really passes (there's
            // no reliable single field for that beyond is_admin/
            // can_manage_admissions) and whether MFA needs a code, without a
            // separate admin-only endpoint an admissions moderator can't reach.
            if (scope === "full") {
                await getStats(mfaCode || undefined);
            } else {
                await listAdminAdmissionPrograms({ limit: 1 }, mfaCode || undefined);
            }
            setStatus({ name: "ready", account, scope });
        } catch (cause) {
            if (cause instanceof ApiError) {
                if (cause.status === 403) {
                    setStatus({ name: "forbidden" });
                    return;
                }
                if (cause.status === 428) {
                    setStatus({ name: "mfa-required" });
                    return;
                }
                if (cause.status === 429) {
                    setStatus({ name: "rate-limited" });
                    return;
                }
                setStatus({ name: "error", message: cause.message });
                return;
            }
            setStatus({ name: "error", message: "發生未知錯誤，請稍後再試。" });
        }
    }

    useEffect(() => {
        // probe() only calls setState after its awaited fetches resolve, never
        // synchronously, so this can't cascade — safe to call directly. Only
        // re-probe when the verified MFA code or the current page changes (an
        // admissions-only account needs re-checking if it navigates, since
        // that's when the scope-mismatch redirect above applies).
        // eslint-disable-next-line react-hooks/set-state-in-effect
        void probe();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [mfaCode, pathname]);

    async function handleMfaSubmit(event: React.FormEvent) {
        event.preventDefault();
        setMfaSubmitting(true);
        setMfaError(null);
        try {
            await verifyAdminMfa(mfaInput.trim());
            setMfaCode(mfaInput.trim());
            setMfaInput("");
        } catch (cause) {
            setMfaError(cause instanceof ApiError ? cause.message : "驗證失敗，請稍後再試。");
        } finally {
            setMfaSubmitting(false);
        }
    }

    async function handleLogout() {
        try {
            await logout();
        } finally {
            window.location.href = "/login";
        }
    }

    if (status.name === "checking") {
        return (
            <CenteredMessage
                icon={<Loader2 aria-hidden className="h-8 w-8 animate-spin text-ink/40" />}
            >
                確認管理員權限中…
            </CenteredMessage>
        );
    }

    if (status.name === "signed-out") {
        return (
            <CenteredMessage icon={<ShieldAlert aria-hidden className="h-10 w-10 text-ink/40" />}>
                <p className="font-serif text-2xl text-ink">請先登入</p>
                <p className="mt-2 font-sans text-copy-muted">
                    管理後台需要以管理員帳號登入才能使用。
                </p>
                <Button asChild className="mt-6">
                    <Link href="/login">前往登入</Link>
                </Button>
            </CenteredMessage>
        );
    }

    if (status.name === "forbidden") {
        return (
            <CenteredMessage icon={<ShieldAlert aria-hidden className="h-10 w-10 text-ink/40" />}>
                <p className="font-serif text-2xl text-ink">沒有管理員權限</p>
                <p className="mt-2 font-sans text-copy-muted">這個帳號沒有管理後台的存取權限。</p>
                <Button
                    asChild
                    className="mt-6 border border-ink/15 bg-surface text-ink hover:bg-ink/5"
                >
                    <Link href="/">回到首頁</Link>
                </Button>
            </CenteredMessage>
        );
    }

    if (status.name === "rate-limited") {
        return (
            <CenteredMessage icon={<Lock aria-hidden className="h-10 w-10 text-ink/40" />}>
                <p className="font-serif text-2xl text-ink">驗證嘗試次數過多</p>
                <p className="mt-2 font-sans text-copy-muted">
                    管理員雙重驗證錯誤次數過多，帳號已暫時鎖定。請稍後再試。
                </p>
            </CenteredMessage>
        );
    }

    if (status.name === "error") {
        return (
            <CenteredMessage icon={<ShieldAlert aria-hidden className="h-10 w-10 text-ink/40" />}>
                <p className="font-serif text-2xl text-ink">發生錯誤</p>
                <p className="mt-2 font-sans text-copy-muted">{status.message}</p>
                <Button className="mt-6" onClick={() => void probe()}>
                    重新整理
                </Button>
            </CenteredMessage>
        );
    }

    if (status.name === "mfa-required") {
        return (
            <CenteredMessage icon={<Lock aria-hidden className="h-10 w-10 text-ink/40" />}>
                <p className="font-serif text-2xl text-ink">需要雙重驗證</p>
                <p className="mt-2 font-sans text-copy-muted">
                    請輸入驗證器 App 上的 6 位數代碼，驗證通過後短時間內不需要再次輸入。
                </p>
                <form onSubmit={handleMfaSubmit} className="mt-6 flex flex-col items-center gap-3">
                    <input
                        className="w-40 rounded-[var(--radius-small)] border border-ink/15 bg-surface px-4 py-3 text-center font-sans text-lg tracking-[0.3em] text-ink outline-none focus:border-ink/40"
                        inputMode="numeric"
                        pattern="[0-9]{6}"
                        maxLength={6}
                        autoComplete="one-time-code"
                        value={mfaInput}
                        onChange={(e) => setMfaInput(e.target.value.replace(/\D/g, ""))}
                        required
                    />
                    {mfaError ? <p className="font-sans text-sm text-red-600">{mfaError}</p> : null}
                    <Button type="submit" disabled={mfaSubmitting || mfaInput.length !== 6}>
                        {mfaSubmitting ? "驗證中…" : "驗證"}
                    </Button>
                </form>
            </CenteredMessage>
        );
    }

    const navItems = status.scope === "full" ? fullNavItems : admissionsOnlyNavItems;

    return (
        <AdminContextProvider value={{ account: status.account, mfaCode }}>
            <div className="min-h-svh bg-surface">
                <header className="sticky top-0 z-50 w-full border-b border-ink/10 bg-surface/95 backdrop-blur-sm">
                    <div className="mx-auto flex max-w-screen-xl items-center justify-between gap-4 px-4 py-3 sm:px-6 sm:py-4 lg:px-16">
                        <Link
                            href="/"
                            className="flex min-w-0 items-center gap-3 pr-2 lg:flex-none lg:pr-0"
                        >
                            <Image
                                src={logoIcon}
                                alt="S.T.A Logo"
                                width={32}
                                height={32}
                                className="shrink-0"
                            />
                            <span className="min-w-0 font-serif text-lg leading-tight font-normal tracking-[-0.02em] text-ink sm:text-2xl lg:text-brand lg:whitespace-nowrap">
                                <span>S.T.A</span>
                                <span className="mx-2 hidden sm:inline">|</span>
                                <span className="block sm:inline">管理後台</span>
                            </span>
                        </Link>

                        <nav className="hidden items-center gap-7 lg:flex">
                            {navItems.map((item) => (
                                <Link
                                    key={item.href}
                                    href={item.href}
                                    className={twMerge(
                                        "border-b-2 border-transparent py-2 font-sans text-base transition-colors",
                                        pathname === item.href
                                            ? "border-[#f6bd42] text-ink"
                                            : "text-ink/70 hover:text-ink"
                                    )}
                                >
                                    {item.label}
                                </Link>
                            ))}
                            <Link
                                href="/"
                                className="flex items-center gap-1.5 font-sans text-base text-ink/70 transition-colors hover:text-ink"
                            >
                                <ArrowLeft aria-hidden className="h-4 w-4" />
                                回到前台
                            </Link>
                            <div className="flex items-center gap-2 border-l border-ink/15 pl-5">
                                <span className="max-w-28 truncate font-sans text-sm text-ink/60">
                                    {status.account.username}
                                </span>
                                <button
                                    type="button"
                                    onClick={handleLogout}
                                    aria-label="登出"
                                    className="rounded-[var(--radius-small)] p-1.5 text-ink/60 transition-colors hover:bg-ink/5 hover:text-ink"
                                >
                                    <LogOut aria-hidden className="h-4 w-4" />
                                </button>
                            </div>
                        </nav>

                        <button
                            type="button"
                            onClick={handleLogout}
                            className="font-sans text-sm text-ink/60 hover:text-ink lg:hidden"
                        >
                            登出
                        </button>
                    </div>
                    <nav className="mx-auto flex max-w-screen-xl gap-6 overflow-x-auto px-4 pb-3 sm:px-6 lg:hidden">
                        {navItems.map((item) => (
                            <Link
                                key={item.href}
                                href={item.href}
                                className={twMerge(
                                    "shrink-0 border-b-2 border-transparent pb-1 font-sans text-sm",
                                    pathname === item.href
                                        ? "border-[#f6bd42] text-ink"
                                        : "text-ink/60"
                                )}
                            >
                                {item.label}
                            </Link>
                        ))}
                        <Link href="/" className="shrink-0 font-sans text-sm text-ink/60">
                            回到前台
                        </Link>
                    </nav>
                </header>
                <main className="mx-auto flex min-h-[calc(100svh-5rem)] w-full max-w-screen-xl flex-1 flex-col px-5 py-8 sm:px-6 lg:px-16 lg:py-10">
                    {children}
                </main>
            </div>
        </AdminContextProvider>
    );
}

function CenteredMessage({ icon, children }: { icon: React.ReactNode; children: React.ReactNode }) {
    return (
        <div className="flex min-h-svh flex-col items-center justify-center bg-surface px-6 text-center">
            {icon}
            <div className="mt-4">{children}</div>
        </div>
    );
}
