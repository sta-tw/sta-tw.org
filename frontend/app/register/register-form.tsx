"use client";

import Link from "next/link";
import { useState } from "react";
import { Label } from "radix-ui";
import { ShieldCheck } from "lucide-react";
import Button from "../components/button";

const focusStyle =
    "focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ink";
const inputStyle =
    "h-12 w-full rounded-xl border border-ink/25 bg-white/70 px-4 text-base placeholder:text-ink/45 focus:border-ink focus:outline-2 focus:outline-offset-2 focus:outline-accent-green-strong";

export default function RegisterForm() {
    const [notice, setNotice] = useState("");
    const [password, setPassword] = useState("");
    const [confirmation, setConfirmation] = useState("");
    const [mismatch, setMismatch] = useState(false);

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
                    if (password !== confirmation) {
                        setMismatch(true);
                        return;
                    }
                    setMismatch(false);
                    setNotice("註冊功能尚未開放，敬請期待。");
                }}
            >
                <div className="space-y-4">
                    <div className="flex flex-col gap-2">
                        <Label.Root
                            htmlFor="register-email"
                            className="text-base font-medium text-ink"
                        >
                            帳號
                        </Label.Root>
                        <input
                            id="register-email"
                            name="email"
                            type="email"
                            autoComplete="email"
                            placeholder="常用信箱"
                            required
                            className={inputStyle}
                        />
                    </div>
                    <div className="flex flex-col gap-2">
                        <Label.Root
                            htmlFor="register-nickname"
                            className="text-base font-medium text-ink"
                        >
                            暱稱（公開顯示）
                        </Label.Root>
                        <input
                            id="register-nickname"
                            name="nickname"
                            autoComplete="nickname"
                            placeholder="不可帶有除 . 與 _ 之外之特殊符號"
                            pattern={"[\\p{L}\\p{N}._]+"}
                            title="請使用文字、數字、半形句點或底線"
                            required
                            className={inputStyle}
                        />
                    </div>
                    <div className="flex flex-col gap-2">
                        <Label.Root
                            htmlFor="register-password"
                            className="text-base font-medium text-ink"
                        >
                            密碼
                        </Label.Root>
                        <input
                            id="register-password"
                            name="password"
                            type="password"
                            autoComplete="new-password"
                            placeholder="5 位以上英數組合"
                            required
                            minLength={5}
                            pattern="(?=.*[A-Za-z])(?=.*[0-9])[A-Za-z0-9]{5,}"
                            title="請輸入至少 5 位，包含英文字母與數字的密碼"
                            value={password}
                            onChange={(event) => {
                                setPassword(event.target.value);
                                setMismatch(false);
                            }}
                            className={inputStyle}
                        />
                    </div>
                    <div className="flex flex-col gap-2">
                        <Label.Root
                            htmlFor="register-confirmation"
                            className="text-base font-medium text-ink"
                        >
                            再次輸入密碼
                        </Label.Root>
                        <input
                            id="register-confirmation"
                            name="confirmation"
                            type="password"
                            autoComplete="new-password"
                            placeholder="再次輸入密碼"
                            required
                            value={confirmation}
                            onChange={(event) => {
                                setConfirmation(event.target.value);
                                setMismatch(false);
                            }}
                            aria-invalid={mismatch}
                            aria-describedby={mismatch ? "password-error" : undefined}
                            className={inputStyle}
                        />
                        {mismatch && (
                            <p id="password-error" role="alert" className="text-sm text-red-700">
                                兩次輸入的密碼不一致。
                            </p>
                        )}
                    </div>
                </div>
                <Button
                    type="submit"
                    className={`mt-6 h-12 w-full rounded-xl bg-accent-green font-sans text-lg text-ink hover:bg-accent-green-strong active:bg-accent-green-strong ${focusStyle}`}
                >
                    註冊
                </Button>
                <div className="mt-4 flex min-h-16 items-center justify-center gap-3 rounded-xl border border-ink/10 bg-ink/5 px-4 py-3 text-copy-muted">
                    <ShieldCheck size={22} aria-hidden="true" className="shrink-0" />
                    <div>
                        <p className="text-sm font-medium">Cloudflare 驗證</p>
                        <p className="mt-0.5 text-xs">驗證區塊預留・尚未啟用</p>
                    </div>
                </div>
                <p className="mt-4 text-center text-sm leading-relaxed text-copy-muted">
                    註冊即表示您同意{" "}
                    <button
                        type="button"
                        onClick={() => setNotice("服務條款尚未公布。")}
                        className={`cursor-pointer font-bold text-ink underline underline-offset-4 ${focusStyle}`}
                    >
                        服務條款
                    </button>
                    {" 與 "}
                    <button
                        type="button"
                        onClick={() => setNotice("隱私權政策尚未公布。")}
                        className={`cursor-pointer font-bold text-ink underline underline-offset-4 ${focusStyle}`}
                    >
                        隱私權政策
                    </button>
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
