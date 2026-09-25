"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { Label } from "radix-ui";
import Button from "../components/button";
import { confirmPasswordReset } from "../lib/api/auth";
import { ApiError } from "../lib/api/types";

const focusStyle =
    "focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ink";
const inputStyle =
    "h-12 w-full rounded-xl border border-ink/25 bg-white/70 px-4 text-base placeholder:text-ink/45 focus:border-ink focus:outline-2 focus:outline-offset-2 focus:outline-accent-green-strong";

function errorMessage(error: unknown): string {
    if (error instanceof ApiError) {
        switch (error.code) {
            case "invalid_token":
                return "這個連結已失效或過期，請重新註冊或重新申請重設密碼。";
            case "invalid_request":
                return "密碼格式不正確，請確認至少 12 位。";
            case "network_error":
                return error.message;
            default:
                return "設定失敗，請稍後再試。";
        }
    }
    return "設定失敗，請稍後再試。";
}

export default function ResetPasswordForm() {
    const [token, setToken] = useState("");
    const [notice, setNotice] = useState("");
    const [password, setPassword] = useState("");
    const [confirmation, setConfirmation] = useState("");
    const [mismatch, setMismatch] = useState(false);
    const [submitting, setSubmitting] = useState(false);
    const [done, setDone] = useState(false);

    useEffect(() => {
        const params = new URLSearchParams(window.location.search);
        // eslint-disable-next-line react-hooks/set-state-in-effect
        setToken(params.get("token") ?? "");
    }, []);

    if (done) {
        return (
            <div className="w-full max-w-md text-center">
                <h1 id="reset-password-title" className="mb-4 text-3xl leading-tight font-medium text-ink sm:text-4xl">
                    密碼設定完成
                </h1>
                <p className="text-base leading-relaxed text-copy-muted">帳號已經可以用了，用新密碼登入吧。</p>
                <Link
                    href="/login"
                    className={`mt-6 inline-block font-bold text-ink underline underline-offset-4 ${focusStyle}`}
                >
                    前往登入
                </Link>
            </div>
        );
    }

    return (
        <div className="w-full max-w-md">
            <h1
                id="reset-password-title"
                className="mb-8 text-center text-3xl leading-tight font-medium text-ink sm:text-4xl"
            >
                設定密碼
            </h1>
            <form
                onSubmit={(event) => {
                    event.preventDefault();
                    if (submitting) return;
                    if (!token) {
                        setNotice("連結缺少必要參數，請確認網址完整。");
                        return;
                    }
                    if (password !== confirmation) {
                        setMismatch(true);
                        return;
                    }
                    setMismatch(false);
                    setNotice("");
                    setSubmitting(true);
                    confirmPasswordReset(token, password)
                        .then(() => setDone(true))
                        .catch((error: unknown) => setNotice(errorMessage(error)))
                        .finally(() => setSubmitting(false));
                }}
            >
                <div className="space-y-4">
                    <div className="flex flex-col gap-2">
                        <Label.Root htmlFor="reset-password" className="text-base font-medium text-ink">
                            新密碼
                        </Label.Root>
                        <input
                            id="reset-password"
                            name="password"
                            type="password"
                            autoComplete="new-password"
                            placeholder="12 位以上英數組合"
                            required
                            minLength={12}
                            maxLength={128}
                            title="請輸入至少 12 位的密碼"
                            value={password}
                            onChange={(event) => {
                                setPassword(event.target.value);
                                setMismatch(false);
                            }}
                            className={inputStyle}
                        />
                    </div>
                    <div className="flex flex-col gap-2">
                        <Label.Root htmlFor="reset-password-confirmation" className="text-base font-medium text-ink">
                            再次輸入新密碼
                        </Label.Root>
                        <input
                            id="reset-password-confirmation"
                            name="confirmation"
                            type="password"
                            autoComplete="new-password"
                            placeholder="再次輸入新密碼"
                            required
                            value={confirmation}
                            onChange={(event) => {
                                setConfirmation(event.target.value);
                                setMismatch(false);
                            }}
                            aria-invalid={mismatch}
                            aria-describedby={mismatch ? "reset-password-error" : undefined}
                            className={inputStyle}
                        />
                        {mismatch && (
                            <p id="reset-password-error" role="alert" className="text-sm text-red-700">
                                兩次輸入的密碼不一致。
                            </p>
                        )}
                    </div>
                </div>
                <Button
                    type="submit"
                    disabled={submitting}
                    className={`mt-6 h-12 w-full rounded-xl bg-accent-green font-sans text-lg text-ink hover:bg-accent-green-strong active:bg-accent-green-strong disabled:cursor-not-allowed disabled:opacity-60 ${focusStyle}`}
                >
                    {submitting ? "設定中…" : "設定密碼"}
                </Button>
            </form>
            <p role="status" aria-live="polite" className="mt-3 text-center text-xs text-ink">
                {notice}
            </p>
        </div>
    );
}
