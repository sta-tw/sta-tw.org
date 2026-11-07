"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import Button from "../components/button";
import Field from "../components/field";
import { Input } from "../components/input";
import { confirmPasswordReset } from "../lib/api/auth";
import { ApiError } from "../lib/api/types";
import RequestResetForm from "./request-reset-form";

function errorMessage(error: unknown): string {
    if (error instanceof ApiError) {
        switch (error.code) {
            case "invalid_token":
                return "這個連結已失效或過期，請重新註冊或重新申請重設密碼。";
            case "invalid_request":
                return "密碼格式不正確，請確認至少 8 位。";
            case "network_error":
                return error.message;
            default:
                return "設定失敗，請稍後再試。";
        }
    }
    return "設定失敗，請稍後再試。";
}

export default function ResetPasswordForm() {
    // null until the query string has been read on the client.
    const [token, setToken] = useState<string | null>(null);
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

    if (token === null) return null;
    if (!token) return <RequestResetForm />;

    if (done) {
        return (
            <div className="w-full max-w-md text-center">
                <h1
                    id="reset-password-title"
                    className="mb-4 text-3xl leading-tight font-medium text-ink sm:text-4xl"
                >
                    密碼設定完成
                </h1>
                <p className="text-base leading-relaxed text-copy-muted">
                    帳號已經可以用了，用新密碼登入吧。
                </p>
                <Link
                    href="/login"
                    className={`mt-6 inline-block font-bold text-ink underline underline-offset-4 focus-ring`}
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
                    <Field label="新密碼">
                        <Input
                            name="password"
                            type="password"
                            autoComplete="new-password"
                            placeholder="8 位以上英數組合"
                            required
                            minLength={8}
                            maxLength={128}
                            title="請輸入至少 8 位的密碼"
                            value={password}
                            onChange={(event) => {
                                setPassword(event.target.value);
                                setMismatch(false);
                            }}
                        />
                    </Field>
                    <Field
                        label="再次輸入新密碼"
                        error={mismatch ? "兩次輸入的密碼不一致。" : undefined}
                    >
                        <Input
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
                        />
                    </Field>
                </div>
                <Button type="submit" disabled={submitting} className="mt-6 w-full">
                    {submitting ? "設定中…" : "設定密碼"}
                </Button>
            </form>
            <p role="status" aria-live="polite" className="mt-3 text-center text-xs text-ink">
                {notice}
            </p>
        </div>
    );
}
