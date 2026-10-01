"use client";

import Link from "next/link";
import { useState } from "react";
import { Label } from "radix-ui";
import Button from "../components/button";
import TurnstileWidget from "../components/turnstile-widget";
import { registerAccount } from "../lib/api/auth";
import { ApiError } from "../lib/api/types";

const focusStyle =
    "focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ink";
const inputStyle =
    "h-12 w-full rounded-xl border border-ink/25 bg-white/70 px-4 text-base placeholder:text-ink/45 focus:border-ink focus:outline-2 focus:outline-offset-2 focus:outline-accent-green-strong";

function registerErrorMessage(error: unknown): string {
    if (error instanceof ApiError) {
        switch (error.code) {
            case "account_conflict":
                return "這個帳號名稱或 Email 已經有人使用了。";
            case "invalid_request":
                return "帳號、Email 或學校信箱格式不正確，學校信箱必須是 *.edu.tw。";
            case "rate_limited":
                return "嘗試次數過多，請稍後再試。";
            case "turnstile_invalid":
                return "驗證未通過，請重新完成驗證。";
            case "turnstile_unavailable":
                return "驗證服務暫時無法使用，請稍後再試。";
            case "network_error":
                return error.message;
            default:
                return "註冊失敗，請稍後再試。";
        }
    }
    return "註冊失敗，請稍後再試。";
}

export default function RegisterForm() {
    const [notice, setNotice] = useState("");
    const [submitting, setSubmitting] = useState(false);
    const [submitted, setSubmitted] = useState(false);
    const [turnstileToken, setTurnstileToken] = useState("");
    const [resetSignal, setResetSignal] = useState(0);

    if (submitted) {
        return (
            <div className="w-full max-w-md text-center">
                <h1
                    id="register-title"
                    className="mb-4 text-3xl leading-tight font-medium text-ink sm:text-4xl"
                >
                    請完成信箱驗證
                </h1>
                <p className="text-base leading-relaxed text-copy-muted">
                    我們寄了一封信到你的學校信箱，裡面有設定密碼的連結。
                    <br />
                    完成設定後帳號就會啟用，之後就能用帳號密碼登入。
                </p>
                <p className="mt-3 text-sm leading-relaxed text-copy-muted">
                    連結 24 小時內有效。沒收到信可以檢查垃圾信匣，或重新註冊一次。
                </p>
                <Link
                    href="/"
                    className={`mt-6 inline-block font-bold text-ink underline underline-offset-4 ${focusStyle}`}
                >
                    回首頁
                </Link>
            </div>
        );
    }

    return (
        <div className="w-full max-w-md">
            <h1
                id="register-title"
                className="mb-8 text-center text-3xl leading-tight font-medium text-ink sm:text-4xl"
            >
                註冊帳號
            </h1>
            <form
                onSubmit={(event) => {
                    event.preventDefault();
                    if (submitting) return;
                    if (!turnstileToken) {
                        setNotice("請先完成 Cloudflare 驗證。");
                        return;
                    }
                    const form = event.currentTarget;
                    const email = (form.elements.namedItem("email") as HTMLInputElement).value;
                    const schoolEmail = (
                        form.elements.namedItem("school_email") as HTMLInputElement
                    ).value;
                    const username = (form.elements.namedItem("nickname") as HTMLInputElement)
                        .value;
                    setNotice("");
                    setSubmitting(true);
                    registerAccount({
                        username,
                        email,
                        school_email: schoolEmail,
                        turnstile_token: turnstileToken
                    })
                        .then(() => setSubmitted(true))
                        .catch((error: unknown) => {
                            setNotice(registerErrorMessage(error));
                            setResetSignal((value) => value + 1);
                        })
                        .finally(() => setSubmitting(false));
                }}
            >
                <div className="space-y-4">
                    <div className="flex flex-col gap-2">
                        <Label.Root
                            htmlFor="register-nickname"
                            className="text-base font-medium text-ink"
                        >
                            帳號名稱
                        </Label.Root>
                        <input
                            id="register-nickname"
                            name="nickname"
                            autoComplete="username"
                            placeholder="不可帶有除 . _ - 之外之特殊符號"
                            pattern={"[\\p{L}\\p{N}._\\-]+"}
                            title="請使用文字、數字、半形句點、底線或連字號"
                            minLength={3}
                            maxLength={64}
                            required
                            className={inputStyle}
                        />
                    </div>
                    <div className="flex flex-col gap-2">
                        <Label.Root
                            htmlFor="register-email"
                            className="text-base font-medium text-ink"
                        >
                            聯絡信箱
                        </Label.Root>
                        <input
                            id="register-email"
                            name="email"
                            type="email"
                            autoComplete="email"
                            placeholder="日常使用的信箱，任何網域都可以"
                            required
                            className={inputStyle}
                        />
                    </div>
                    <div className="flex flex-col gap-2">
                        <Label.Root
                            htmlFor="register-school-email"
                            className="text-base font-medium text-ink"
                        >
                            學校信箱
                        </Label.Root>
                        <input
                            id="register-school-email"
                            name="school_email"
                            type="email"
                            autoComplete="email"
                            placeholder="必須是 *.edu.tw，設定密碼的連結會寄到這裡"
                            pattern={".+@.*\\.edu\\.tw$"}
                            title="請輸入 *.edu.tw 學校信箱"
                            required
                            className={inputStyle}
                        />
                        <p className="text-xs text-copy-muted">
                            兩個信箱可以填一樣的。沒有學校信箱看{" "}
                            <Link
                                href="/faq#no-school-email"
                                className={`font-bold text-ink underline underline-offset-4 ${focusStyle}`}
                            >
                                沒有學校信箱怎麼辦
                            </Link>
                        </p>
                    </div>
                </div>
                <TurnstileWidget
                    action="signup"
                    onTokenChange={setTurnstileToken}
                    resetSignal={resetSignal}
                />
                <Button
                    type="submit"
                    disabled={submitting || !turnstileToken}
                    className={`mt-6 h-12 w-full rounded-xl bg-accent-green font-sans text-lg text-ink hover:bg-accent-green-strong active:bg-accent-green-strong disabled:cursor-not-allowed disabled:opacity-60 ${focusStyle}`}
                >
                    {submitting ? "送出中…" : "註冊"}
                </Button>
                <p className="mt-4 text-center text-sm leading-relaxed text-copy-muted">
                    註冊即表示您同意{" "}
                    <Link
                        href="/terms"
                        className={`cursor-pointer font-bold text-ink underline underline-offset-4 ${focusStyle}`}
                    >
                        服務條款
                    </Link>
                    {" 與 "}
                    <Link
                        href="/privacy"
                        className={`cursor-pointer font-bold text-ink underline underline-offset-4 ${focusStyle}`}
                    >
                        隱私權政策
                    </Link>
                </p>
            </form>
            <p className="mt-4 text-center text-sm text-ink">
                已經有帳號？
                <Link
                    href="/login"
                    className={`font-bold underline-offset-4 hover:underline ${focusStyle}`}
                >
                    登入
                </Link>
            </p>
            <p role="status" aria-live="polite" className="mt-3 text-center text-xs text-ink">
                {notice}
            </p>
        </div>
    );
}
