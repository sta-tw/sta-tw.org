"use client";

import { useEffect, useState } from "react";
import Button from "../../components/button";
import {
    getAdmissionsDisplayLatestYear,
    setAdmissionsDisplayLatestYear
} from "../../lib/api/admin";
import { useAdmin } from "../admin-context";
import { AdminPanel, describeError, ErrorText, inputClass } from "../admin-ui";

export default function DisplayLatestYearControl() {
    const { mfaCode } = useAdmin();
    const [academicYear, setAcademicYear] = useState("");
    const [savedYear, setSavedYear] = useState<number | null>(null);
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [notice, setNotice] = useState<string | null>(null);

    useEffect(() => {
        let ignore = false;
        getAdmissionsDisplayLatestYear(mfaCode || undefined)
            .then((response) => {
                if (ignore) return;
                const year = response.data.academic_year;
                setAcademicYear(year > 0 ? String(year) : "");
                setSavedYear(year > 0 ? year : null);
            })
            .catch((cause: unknown) => {
                if (!ignore) setError(describeError(cause));
            })
            .finally(() => {
                if (!ignore) setLoading(false);
            });
        return () => {
            ignore = true;
        };
    }, [mfaCode]);

    async function save(event: React.FormEvent<HTMLFormElement>) {
        event.preventDefault();
        const year = Number(academicYear);
        if (!/^\d{3}$/.test(academicYear) || year < 100 || year > 999) {
            setError("請輸入 100～999 的三位數學年度。");
            setNotice(null);
            return;
        }

        setSaving(true);
        setError(null);
        setNotice(null);
        try {
            const response = await setAdmissionsDisplayLatestYear(year, mfaCode || undefined);
            setSavedYear(response.data.academic_year);
            setNotice(`簡章搜尋已切換為顯示 ${response.data.academic_year} 學年度資料。`);
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setSaving(false);
        }
    }

    return (
        <AdminPanel>
            <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
                <div className="max-w-3xl">
                    <h2 className="font-serif text-xl text-ink">簡章搜尋顯示最新年度資料</h2>
                    <p className="mt-1 font-sans text-sm leading-6 text-copy-muted">
                        搜尋會優先顯示此年度已上架的校系資料；該校系尚無此年度已上架資料時，會沿用最近一筆較舊年度已上架資料。歷屆資料只列出早於此年度的資料。
                    </p>
                </div>
                <form onSubmit={save} className="flex flex-wrap items-end gap-3">
                    <label className="flex flex-col gap-1 font-sans text-xs font-semibold text-copy-muted">
                        顯示年度
                        <input
                            className={inputClass}
                            inputMode="numeric"
                            maxLength={3}
                            placeholder={loading ? "讀取中" : "例如 117"}
                            value={academicYear}
                            onChange={(event) => setAcademicYear(event.target.value.replace(/\D/g, ""))}
                            aria-label="簡章搜尋顯示最新年度"
                            disabled={loading || saving}
                        />
                    </label>
                    <Button type="submit" className="h-10 px-4 text-sm" disabled={loading || saving}>
                        {saving ? "切換中…" : "切換年度"}
                    </Button>
                </form>
            </div>
            <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 font-sans text-xs text-copy-muted">
                <span>
                    目前設定：{loading ? "讀取中…" : savedYear ? `${savedYear} 學年度` : "尚無資料"}
                </span>
                <span>切換到新年度後，原年度會列入歷屆；各校系的新年度資料上架前仍會顯示舊資料。</span>
            </div>
            {error ? <div className="mt-3"><ErrorText>{error}</ErrorText></div> : null}
            {notice ? <p role="status" className="mt-3 font-sans text-sm text-green-700">{notice}</p> : null}
        </AdminPanel>
    );
}
