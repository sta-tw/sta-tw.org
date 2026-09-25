"use client";

import Link from "next/link";
import { useState } from "react";
import { Label } from "radix-ui";
import { submitAccountApplication } from "../lib/api/account-applications";
import { ApiError } from "../lib/api/types";

const focusStyle =
    "focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ink";
const inputStyle =
    "w-full rounded-xl border border-ink/25 bg-white/70 px-4 py-3 text-base placeholder:text-ink/45 focus:border-ink focus:outline-2 focus:outline-offset-2 focus:outline-accent-green-strong";

function submitErrorMessage(error: unknown): string {
    if (error instanceof ApiError) {
        switch (error.code) {
            case "invalid_input":
                return "帳號名稱或信箱格式不正確，請確認後再試一次。";
            case "rate_limited":
                return "申請次數過多，請稍後再試。";
            case "network_error":
                return error.message;
            default:
                return "送出失敗，請稍後再試。";
        }
    }
    return "送出失敗，請稍後再試。";
}

export default function ApplyForm() {
    const [notice, setNotice] = useState("");
    const [submitting, setSubmitting] = useState(false);
    const [submitted, setSubmitted] = useState(false);

    if (submitted) {
        return (
            <div className="w-full max-w-md text-center">
                <h1 id="apply-title" className="mb-4 text-3xl leading-tight font-medium text-ink sm:text-4xl">
                    申請已送出
                </h1>
                <p className="text-base leading-relaxed text-copy-muted">
                    管理員審核通過後，會寄一封信到你留的信箱，附上設定密碼的連結，設定完成後帳號就會啟用。審核需要人工處理時間，請耐心等候；有問題可以直接回信到那封通知信，我們會收到。
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
                id="apply-title"
                className="mb-3 text-center text-3xl leading-tight font-medium text-ink sm:text-4xl"
            >
                沒有學校信箱？申請帳號
            </h1>
            <p className="mb-8 text-center text-sm leading-relaxed text-copy-muted">
                一般註冊需要 *.edu.tw 學校信箱。沒有的話，填這個表單申請帳號，管理員審核通過後會寄信通知你。
            </p>
            <form
                onSubmit={(event) => {
                    event.preventDefault();
                    if (submitting) return;
                    const form = event.currentTarget;
                    const username = (form.elements.namedItem("username") as HTMLInputElement).value;
                    const email = (form.elements.namedItem("email") as HTMLInputElement).value;
                    const note = (form.elements.namedItem("note") as HTMLTextAreaElement).value;
                    const filesInput = form.elements.namedItem("files") as HTMLInputElement;
                    const files = filesInput.files ? Array.from(filesInput.files) : [];
                    setNotice("");
                    setSubmitting(true);
                    submitAccountApplication({ username, email, note, files })
                        .then(() => setSubmitted(true))
                        .catch((error: unknown) => setNotice(submitErrorMessage(error)))
                        .finally(() => setSubmitting(false));
                }}
            >
                <div className="space-y-4">
                    <div className="flex flex-col gap-2">
                        <Label.Root htmlFor="apply-username" className="text-base font-medium text-ink">
                            想要的帳號名稱
                        </Label.Root>
                        <input
                            id="apply-username"
                            name="username"
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
                        <Label.Root htmlFor="apply-email" className="text-base font-medium text-ink">
                            聯絡信箱
                        </Label.Root>
                        <input
                            id="apply-email"
                            name="email"
                            type="email"
                            autoComplete="email"
                            placeholder="審核結果與設定密碼的連結會寄到這裡"
                            required
                            className={inputStyle}
                        />
                    </div>
                    <div className="flex flex-col gap-2">
                        <Label.Root htmlFor="apply-note" className="text-base font-medium text-ink">
                            申請說明
                        </Label.Root>
                        <textarea
                            id="apply-note"
                            name="note"
                            rows={4}
                            placeholder="簡單說明你的情形，例如自學生、無法取得學校信箱的原因等"
                            required
                            className={`${inputStyle} resize-y`}
                        />
                    </div>
                    <div className="flex flex-col gap-2">
                        <Label.Root htmlFor="apply-files" className="text-base font-medium text-ink">
                            佐證資料（可選，可多選）
                        </Label.Root>
                        <input
                            id="apply-files"
                            name="files"
                            type="file"
                            multiple
                            className={`${inputStyle} file:mr-4 file:rounded-lg file:border-0 file:bg-ink/10 file:px-3 file:py-2 file:text-sm file:font-medium file:text-ink`}
                        />
                    </div>
                </div>
                <button
                    type="submit"
                    disabled={submitting}
                    className={`mt-6 h-12 w-full rounded-xl bg-accent-green font-sans text-lg text-ink hover:bg-accent-green-strong active:bg-accent-green-strong disabled:cursor-not-allowed disabled:opacity-60 ${focusStyle}`}
                >
                    {submitting ? "送出中…" : "送出申請"}
                </button>
            </form>
            <p className="mt-4 text-center text-sm text-ink">
                有學校信箱？
                <Link href="/register" className={`font-bold underline-offset-4 hover:underline ${focusStyle}`}>
                    直接註冊
                </Link>
            </p>
            <p role="status" aria-live="polite" className="mt-3 text-center text-xs text-ink">
                {notice}
            </p>
        </div>
    );
}
