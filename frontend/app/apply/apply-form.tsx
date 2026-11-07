"use client";

import Link from "next/link";
import { useState } from "react";
import Button from "../components/button";
import Field from "../components/field";
import FileUpload from "../components/file-upload";
import { Input, Textarea } from "../components/input";
import { submitAccountApplication } from "../lib/api/account-applications";
import { ApiError } from "../lib/api/types";

function submitErrorMessage(error: unknown): string {
    if (error instanceof ApiError) {
        switch (error.code) {
            case "invalid_input":
                return "帳號名稱或信箱格式不正確，請確認後再試一次。";
            case "rate_limited":
                return "申請次數過多，請稍後再試。";
            case "review_unavailable":
                return "目前無法受理帳號申請，請稍後再試。";
            case "email_unavailable":
                return "目前無法寄送申請通知信，請稍後再試。";
            case "notification_failed":
                return "收件確認信已寄出，但通知審核人員時發生問題。請先不要重複送出，稍後聯絡管理員。";
            case "receipt_email_failed":
                return "申請資料已收到，但收件確認信寄送失敗。請先不要重複送出，稍後聯絡管理員。";
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
    const [files, setFiles] = useState<File[]>([]);

    if (submitted) {
        return (
            <div className="w-full max-w-md text-center">
                <h1
                    id="apply-title"
                    className="mb-4 text-3xl leading-tight font-medium text-ink sm:text-4xl"
                >
                    申請已送出
                </h1>
                <p className="text-base leading-relaxed text-copy-muted">
                    收件確認信已寄到你留的信箱。管理員審核通過後，會再寄一封帳號啟用信，附上設定密碼的連結；有補充資料可直接回覆收件確認信，我們會收到。
                </p>
                <Link
                    href="/"
                    className={`mt-6 inline-block font-bold text-ink underline underline-offset-4 focus-ring`}
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
                一般註冊需要 *.edu.tw
                學校信箱。沒有的話，填這個表單申請帳號，管理員審核通過後會寄信通知你。
            </p>
            <form
                onSubmit={(event) => {
                    event.preventDefault();
                    if (submitting) return;
                    const form = event.currentTarget;
                    const username = (form.elements.namedItem("username") as HTMLInputElement)
                        .value;
                    const email = (form.elements.namedItem("email") as HTMLInputElement).value;
                    const note = (form.elements.namedItem("note") as HTMLTextAreaElement).value;
                    setNotice("");
                    setSubmitting(true);
                    submitAccountApplication({ username, email, note, files })
                        .then(() => setSubmitted(true))
                        .catch((error: unknown) => setNotice(submitErrorMessage(error)))
                        .finally(() => setSubmitting(false));
                }}
            >
                <div className="space-y-4">
                    <Field label="想要的帳號名稱">
                        <Input
                            name="username"
                            autoComplete="username"
                            placeholder="不可帶有除 . _ - 之外之特殊符號"
                            pattern={"[\\p{L}\\p{N}._\\-]+"}
                            title="請使用文字、數字、半形句點、底線或連字號"
                            minLength={3}
                            maxLength={64}
                            required
                        />
                    </Field>
                    <Field label="聯絡信箱">
                        <Input
                            name="email"
                            type="email"
                            autoComplete="email"
                            placeholder="審核結果與設定密碼的連結會寄到這裡"
                            required
                        />
                    </Field>
                    <Field label="申請說明">
                        <Textarea
                            name="note"
                            rows={4}
                            placeholder="簡單說明你的情形，例如自學生、無法取得學校信箱的原因等"
                            required
                        />
                    </Field>
                    <Field label="佐證資料（可選，可多選）">
                        <FileUpload
                            multiple
                            onFilesSelected={setFiles}
                            fileName={files.map((file) => file.name).join("、") || undefined}
                        />
                    </Field>
                </div>
                <Button type="submit" disabled={submitting} className="mt-6 w-full">
                    {submitting ? "送出中…" : "送出申請"}
                </Button>
            </form>
            <p className="mt-4 text-center text-sm text-ink">
                有學校信箱？
                <Link
                    href="/register"
                    className={`font-bold underline-offset-4 focus-ring hover:underline`}
                >
                    直接註冊
                </Link>
            </p>
            <p role="status" aria-live="polite" className="mt-3 text-center text-xs text-ink">
                {notice}
            </p>
        </div>
    );
}
