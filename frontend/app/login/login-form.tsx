"use client";

import Image from "next/image";
import Link from "next/link";
import { useState } from "react";
import { Label } from "radix-ui";
import Button from "../components/button";
import { publicPath } from "../lib/public-path";

const focusStyle =
    "focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ink";
const inputStyle =
    "h-12 w-full rounded-xl border border-ink/25 bg-white/70 px-4 text-base placeholder:text-ink/45 focus:border-ink focus:outline-2 focus:outline-offset-2 focus:outline-accent-green-strong";

export default function LoginForm() {
    const [notice, setNotice] = useState("");

    function showPreviewNotice(action: string) {
        setNotice(`${action}功能尚未開放，敬請期待。`);
    }

    return (
        <div className="relative w-full max-w-md">
            <h1
                id="login-title"
                className="mb-8 text-center text-3xl leading-tight font-medium text-ink sm:text-4xl"
            >
                登入帳號
            </h1>

            <div className="flex flex-col gap-3">
                {(["Google", "Discord"] as const).map((provider) => (
                    <Button
                        key={provider}
                        type="button"
                        onClick={() => showPreviewNotice(`${provider} 登入`)}
                        className={`h-12 w-full gap-3 rounded-xl border border-ink/25 bg-white/70 font-sans text-base font-medium text-ink hover:bg-ink/5 active:bg-ink/10 ${focusStyle}`}
                    >
                        <span
                            className={`flex size-7 items-center justify-center rounded-sm ${provider === "Discord" ? "bg-[#5865f2]" : "bg-white"}`}
                        >
                            <Image
                                src={publicPath(`/login/${provider.toLowerCase()}.svg`)}
                                alt=""
                                width={20}
                                height={20}
                            />
                        </span>
                        使用 {provider} 登入
                    </Button>
                ))}
            </div>

            <div className="my-6 flex items-center gap-4 text-sm text-copy-muted">
                <span className="h-px flex-1 bg-ink/20" />
                <span>或者以 Email 登入</span>
                <span className="h-px flex-1 bg-ink/20" />
            </div>

            <form
                onSubmit={(event) => {
                    event.preventDefault();
                    showPreviewNotice("登入");
                }}
            >
                <div className="mb-4 flex flex-col gap-2">
                    <Label.Root htmlFor="login-email" className="text-base font-medium text-ink">
                        帳號
                    </Label.Root>
                    <input
                        id="login-email"
                        name="email"
                        type="email"
                        autoComplete="username"
                        placeholder="常用信箱"
                        required
                        className={inputStyle}
                    />
                </div>
                <div className="flex flex-col gap-2">
                    <Label.Root htmlFor="login-password" className="text-base font-medium text-ink">
                        密碼
                    </Label.Root>
                    <input
                        id="login-password"
                        name="password"
                        type="password"
                        autoComplete="current-password"
                        placeholder="5 位以上英數組合"
                        required
                        minLength={5}
                        className={inputStyle}
                    />
                </div>
                <Button
                    type="submit"
                    className={`mt-6 h-12 w-full rounded-xl bg-accent-green font-sans text-lg text-ink hover:bg-accent-green-strong active:bg-accent-green-strong ${focusStyle}`}
                >
                    登入
                </Button>
            </form>

            <div className="mt-5 flex flex-wrap items-center justify-center gap-x-3 gap-y-2 text-sm text-ink">
                <button
                    type="button"
                    onClick={() => showPreviewNotice("忘記密碼")}
                    className={`cursor-pointer font-bold underline-offset-4 hover:underline ${focusStyle}`}
                >
                    忘記密碼
                </button>
                <span aria-hidden="true" className="h-4 w-px bg-ink/25" />
                <span>
                    沒有帳號？
                    <Link
                        href="/register"
                        className={`cursor-pointer font-bold underline-offset-4 hover:underline ${focusStyle}`}
                    >
                        註冊
                    </Link>
                </span>
            </div>
            <p
                role="status"
                aria-live="polite"
                className="absolute top-full right-0 left-0 mt-3 text-center text-xs text-ink"
            >
                {notice}
            </p>
        </div>
    );
}
