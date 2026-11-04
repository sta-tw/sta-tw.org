"use client";

import Link from "next/link";
import { useState } from "react";
import Button from "../components/button";
import Field from "../components/field";
import { Input } from "../components/input";
import { ErrorText } from "../components/status";
import { requestPasswordReset } from "../lib/api/auth";
import { ApiError } from "../lib/api/types";

/** /reset-password without a token: asks for the account email and mails a
 * reset link. The backend answers the same way whether or not the address
 * matches an account, and so does this form. */
export default function RequestResetForm() {
    const [email, setEmail] = useState("");
    const [notice, setNotice] = useState("");
    const [submitting, setSubmitting] = useState(false);
    const [sent, setSent] = useState(false);

    if (sent) {
        return (
            <div className="w-full max-w-md text-center">
                <h1
                    id="reset-password-title"
                    className="mb-4 text-3xl leading-tight font-medium text-ink sm:text-4xl"
                >
                    請查看信箱
                </h1>
                <p className="text-base leading-relaxed text-copy-muted">
                    如果 {email}{" "}
                    對應到已啟用的帳號，我們已寄出重設密碼連結。沒收到的話，請檢查垃圾郵件匣，或稍候幾分鐘再試一次。
                </p>
                <Link
                    href="/login"
                    className={`mt-6 inline-block font-bold text-ink underline underline-offset-4 focus-ring`}
                >
                    回到登入
                </Link>
            </div>
        );
    }

    return (
        <div className="w-full max-w-md">
            <h1
                id="reset-password-title"
                className="mb-3 text-center text-3xl leading-tight font-medium text-ink sm:text-4xl"
            >
                忘記密碼
            </h1>
            <p className="mb-8 text-center text-sm leading-relaxed text-copy-muted">
                輸入註冊時填寫的電子郵件，我們會寄送重設密碼的連結。
            </p>
            <form
                onSubmit={(event) => {
                    event.preventDefault();
                    if (submitting) return;
                    setNotice("");
                    setSubmitting(true);
                    requestPasswordReset(email.trim())
                        .then(() => setSent(true))
                        .catch((error: unknown) =>
                            setNotice(
                                error instanceof ApiError && error.code === "network_error"
                                    ? error.message
                                    : "目前無法送出，請稍後再試。"
                            )
                        )
                        .finally(() => setSubmitting(false));
                }}
            >
                <Field label="電子郵件">
                    <Input
                        name="email"
                        type="email"
                        autoComplete="email"
                        placeholder="name@example.com"
                        required
                        value={email}
                        onChange={(event) => setEmail(event.target.value)}
                    />
                </Field>
                <Button type="submit" disabled={submitting} className="mt-6 w-full">
                    {submitting ? "送出中…" : "寄送重設連結"}
                </Button>
            </form>
            <div className="mt-3 text-center">
                <ErrorText>{notice}</ErrorText>
            </div>
            <p className="mt-4 text-center text-sm text-ink">
                想起密碼了？
                <Link
                    href="/login"
                    className={`font-bold underline-offset-4 focus-ring hover:underline`}
                >
                    回到登入
                </Link>
            </p>
        </div>
    );
}
