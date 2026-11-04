"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";
import { oauthProviderIcons } from "../components/brand-icons";
import Button from "../components/button";
import Field from "../components/field";
import { Input } from "../components/input";
import TurnstileWidget from "../components/turnstile-widget";
import { login } from "../lib/api/auth";
import { oauthErrorMessage, oauthLoginUrl, oauthProviders } from "../lib/api/identities";
import { ApiError } from "../lib/api/types";
import { notifyAuthChanged } from "../lib/auth-events";
import { safeNextPath } from "../lib/login-redirect";

const linkStyle = `font-bold underline-offset-4 hover:underline focus-ring`;

function loginErrorMessage(error: unknown): string {
    if (error instanceof ApiError) {
        switch (error.code) {
            case "invalid_credentials":
                return "帳號或密碼不正確。";
            case "rate_limited":
                return "嘗試次數過多，請稍後再試。";
            case "turnstile_invalid":
                return "驗證未通過，請重新完成驗證。";
            case "turnstile_unavailable":
                return "驗證服務暫時無法使用，請稍後再試。";
            case "network_error":
                return error.message;
        }
    }
    return "登入失敗，請稍後再試。";
}

/** Reads ?next= / ?oauth_error= (needs the Suspense boundary in page.tsx). */
export function LoginFormFromParams() {
    const searchParams = useSearchParams();
    const oauthError = searchParams.get("oauth_error");
    return (
        <LoginForm
            next={safeNextPath(searchParams.get("next"))}
            oauthError={
                oauthError ? oauthErrorMessage(oauthError, searchParams.get("provider")) : null
            }
        />
    );
}

export default function LoginForm({
    next = "/",
    oauthError = null
}: {
    next?: string;
    oauthError?: string | null;
}) {
    const router = useRouter();
    const [notice, setNotice] = useState("");
    const [submitting, setSubmitting] = useState(false);
    const [turnstileToken, setTurnstileToken] = useState("");
    const [resetSignal, setResetSignal] = useState(0);

    return (
        <div className="w-full max-w-md">
            <h1
                id="login-title"
                className="mb-8 text-center text-3xl leading-tight font-medium text-ink sm:text-4xl"
            >
                登入帳號
            </h1>

            {oauthError && (
                <p
                    role="alert"
                    className="mb-6 rounded-[var(--radius-small)] border border-danger/30 bg-danger/5 px-4 py-3 text-sm leading-relaxed text-danger"
                >
                    {oauthError}
                </p>
            )}

            <div className="flex flex-col gap-3">
                {oauthProviders.map(({ id, label }) => {
                    const Icon = oauthProviderIcons[id];
                    return (
                        <Button
                            key={id}
                            asChild
                            variant="secondary"
                            size="base"
                            className="w-full gap-3"
                        >
                            <a href={oauthLoginUrl(id, next)}>
                                <Icon className="size-5 shrink-0" />
                                使用 {label} 登入
                            </a>
                        </Button>
                    );
                })}
            </div>

            <div className="my-6 flex items-center gap-4 text-sm text-copy-muted">
                <span aria-hidden className="h-px flex-1 bg-ink/15" />
                或以帳號密碼登入
                <span aria-hidden className="h-px flex-1 bg-ink/15" />
            </div>

            <form
                className="flex flex-col gap-4"
                onSubmit={(event) => {
                    event.preventDefault();
                    if (submitting) return;
                    if (!turnstileToken) {
                        setNotice("請先完成 Cloudflare 驗證。");
                        return;
                    }
                    const form = event.currentTarget;
                    const username = (form.elements.namedItem("username") as HTMLInputElement)
                        .value;
                    const password = (form.elements.namedItem("password") as HTMLInputElement)
                        .value;
                    setNotice("");
                    setSubmitting(true);
                    login({ username, password, turnstile_token: turnstileToken })
                        .then(() => {
                            notifyAuthChanged();
                            router.push(next);
                            router.refresh();
                        })
                        .catch((error: unknown) => {
                            setNotice(loginErrorMessage(error));
                            setResetSignal((value) => value + 1);
                        })
                        .finally(() => setSubmitting(false));
                }}
            >
                <Field label="帳號" required>
                    <Input
                        name="username"
                        type="text"
                        autoComplete="username"
                        placeholder="使用者名稱"
                    />
                </Field>
                <Field label="密碼" required>
                    <Input
                        name="password"
                        type="password"
                        autoComplete="current-password"
                        placeholder="請輸入密碼"
                    />
                </Field>
                <TurnstileWidget
                    action="login"
                    onTokenChange={setTurnstileToken}
                    resetSignal={resetSignal}
                />
                <Button type="submit" disabled={submitting || !turnstileToken} className="w-full">
                    {submitting ? "登入中…" : "登入"}
                </Button>
            </form>

            <p role="status" aria-live="polite" className="mt-3 text-center text-sm text-danger">
                {notice}
            </p>

            <div className="mt-3 flex flex-wrap items-center justify-center gap-x-3 gap-y-2 text-sm text-ink">
                <Link href="/reset-password" className={linkStyle}>
                    忘記密碼
                </Link>
                <span aria-hidden className="h-4 w-px bg-ink/25" />
                <span>
                    沒有帳號？
                    <Link href="/register" className={linkStyle}>
                        註冊
                    </Link>
                </span>
            </div>
        </div>
    );
}
