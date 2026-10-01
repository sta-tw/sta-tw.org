"use client";

import { useEffect, useMemo, useState } from "react";
import Button from "../components/button";
import { PolicyMarkdownPreview } from "../components/policy-document";
import { getAdminSitePolicies, setAdminSitePolicy } from "../lib/api/admin";
import type { SitePolicyKey } from "../lib/api/site-policies";
import { policyDocumentToMarkdown, privacyPolicy, serviceTermsPolicy } from "../lib/site-policies";
import { useAdmin } from "./admin-context";
import { AdminPanel, describeError, ErrorText, LoadingState, textareaClass } from "./admin-ui";

const policyDefaults = {
    terms: serviceTermsPolicy,
    privacy: privacyPolicy
} as const;

const defaultValues = {
    terms: policyDocumentToMarkdown(serviceTermsPolicy),
    privacy: policyDocumentToMarkdown(privacyPolicy)
} satisfies Record<SitePolicyKey, string>;

const policyOptions: Array<{ key: SitePolicyKey; label: string; description: string }> = [
    {
        key: "terms",
        label: "服務條款",
        description: "帳號、平台服務、用戶內容與違規處置規範。"
    },
    {
        key: "privacy",
        label: "隱私權政策",
        description: "資料蒐集、利用、安全措施與使用者權利。"
    }
];

export default function SitePolicyEditor() {
    const { mfaCode } = useAdmin();
    const [activePolicy, setActivePolicy] = useState<SitePolicyKey>("terms");
    const [values, setValues] = useState<Record<SitePolicyKey, string>>(defaultValues);
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [notice, setNotice] = useState<string | null>(null);

    useEffect(() => {
        let ignore = false;
        getAdminSitePolicies(mfaCode || undefined)
            .then((response) => {
                if (ignore) return;
                setValues({
                    terms: response.data.terms.value.trim()
                        ? response.data.terms.value
                        : defaultValues.terms,
                    privacy: response.data.privacy.value.trim()
                        ? response.data.privacy.value
                        : defaultValues.privacy
                });
            })
            .catch((cause) => {
                if (!ignore) setError(describeError(cause));
            })
            .finally(() => {
                if (!ignore) setLoading(false);
            });
        return () => {
            ignore = true;
        };
    }, [mfaCode]);

    const activeOption =
        policyOptions.find((option) => option.key === activePolicy) ?? policyOptions[0];
    const activeValue = values[activePolicy];
    const activeDefault = policyDefaults[activePolicy];
    const preview = useMemo(
        () => <PolicyMarkdownPreview markdown={activeValue} fallback={activeDefault} />,
        [activeDefault, activeValue]
    );

    function updateValue(value: string) {
        setValues((current) => ({ ...current, [activePolicy]: value }));
        setNotice(null);
    }

    async function savePolicy() {
        if (!activeValue.trim() || saving) return;
        setSaving(true);
        setError(null);
        setNotice(null);
        try {
            const response = await setAdminSitePolicy(
                activePolicy,
                activeValue,
                mfaCode || undefined
            );
            setValues((current) => ({ ...current, [activePolicy]: response.value }));
            setNotice(activeOption.label + "已儲存，前台重新整理後會讀取最新內容。");
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setSaving(false);
        }
    }

    return (
        <AdminPanel className="sm:p-6">
            <div className="flex flex-wrap items-start justify-between gap-4">
                <div>
                    <h2 className="font-serif text-xl text-ink">條款維護</h2>
                    <p className="mt-2 max-w-2xl font-sans text-sm leading-6 text-copy-muted">
                        直接編輯服務條款或隱私權政策，右側會即時預覽。內容支援 Markdown
                        標題、粗體、連結與清單。
                    </p>
                </div>
                <span className="rounded-full bg-accent-yellow/70 px-3 py-1 font-sans text-xs font-bold text-ink">
                    管理員限定
                </span>
            </div>

            <div className="mt-5 flex flex-wrap gap-2" role="tablist" aria-label="政策文章">
                {policyOptions.map((option) => (
                    <button
                        key={option.key}
                        type="button"
                        role="tab"
                        aria-selected={activePolicy === option.key}
                        onClick={() => {
                            setActivePolicy(option.key);
                            setError(null);
                            setNotice(null);
                        }}
                        className={
                            activePolicy === option.key
                                ? "rounded-full bg-ink px-4 py-2 font-sans text-sm font-bold text-surface"
                                : "rounded-full border border-ink/15 bg-surface px-4 py-2 font-sans text-sm text-ink/70 hover:border-ink/30 hover:text-ink"
                        }
                    >
                        {option.label}
                    </button>
                ))}
            </div>
            <p className="mt-2 font-sans text-xs text-copy-muted">{activeOption.description}</p>

            {loading ? (
                <LoadingState label="載入政策內容中…" />
            ) : (
                <div className="mt-5 grid min-w-0 gap-5 lg:grid-cols-2">
                    <div className="min-w-0">
                        <label
                            htmlFor="site-policy-editor"
                            className="font-sans text-sm font-bold text-ink"
                        >
                            編輯內容
                        </label>
                        <textarea
                            id="site-policy-editor"
                            value={activeValue}
                            onChange={(event) => updateValue(event.target.value)}
                            className={
                                textareaClass +
                                " mt-2 min-h-[28rem] resize-y font-mono text-xs leading-6"
                            }
                            spellCheck={false}
                            aria-label={activeOption.label + " Markdown 內容"}
                        />
                        <p className="mt-2 text-right font-sans text-xs text-copy-muted">
                            {activeValue.length.toLocaleString("zh-TW")} 字元
                        </p>
                    </div>
                    <div className="min-w-0">
                        <p className="font-sans text-sm font-bold text-ink">即時預覽</p>
                        <div className="mt-2 min-w-0">{preview}</div>
                    </div>
                </div>
            )}

            {error ? (
                <div className="mt-4">
                    <ErrorText>{error}</ErrorText>
                </div>
            ) : null}
            {notice ? <p className="mt-4 font-sans text-sm text-green-700">{notice}</p> : null}

            <div className="mt-5 flex justify-end">
                <Button
                    type="button"
                    onClick={() => void savePolicy()}
                    disabled={loading || saving || !activeValue.trim()}
                    className="h-10 px-5 font-sans text-sm"
                >
                    {saving ? "儲存中…" : "儲存這篇政策"}
                </Button>
            </div>
        </AdminPanel>
    );
}
