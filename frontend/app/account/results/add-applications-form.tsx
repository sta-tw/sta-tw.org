"use client";

import { Plus, Search, TriangleAlert, X } from "lucide-react";
import { useEffect, useState } from "react";
import Button from "../../components/button";
import { Card } from "../../components/card";
import ConfirmDialog from "../../components/confirm-dialog";
import Field from "../../components/field";
import IconButton from "../../components/icon-button";
import { Input } from "../../components/input";
import { searchBrochurePrograms, type BrochureSearchProgram } from "../../lib/api/admissions";
import {
    APPLICATION_BATCH_LIMIT,
    createApplications,
    type Application
} from "../../lib/api/applications";
import { describeError } from "../../lib/describe-error";
import { ErrorText, StatusText } from "../../components/status";
import { useAction } from "../../lib/use-resource";

const SEARCH_DEBOUNCE_MS = 300;

type SearchState =
    | { query: string; status: "loading" }
    | { query: string; status: "error"; error: unknown }
    | { query: string; status: "ready"; results: BrochureSearchProgram[] };

const programLabel = (program: BrochureSearchProgram) =>
    `${program.academic_year} 學年度 ${program.school_name} ${program.admission_program_name}`;

/** Search brochures and register up to 50 programs as (locked) applications. */
export default function AddApplicationsForm({
    existing,
    onCreated
}: {
    existing: Application[];
    onCreated: (created: Application[]) => void;
}) {
    const [query, setQuery] = useState("");
    const [search, setSearch] = useState<SearchState | null>(null);
    const [selected, setSelected] = useState<BrochureSearchProgram[]>([]);
    const { pending, error, run } = useAction();
    const submitting = pending !== null;

    const trimmed = query.trim();

    useEffect(() => {
        if (!trimmed) return;
        const controller = new AbortController();
        const timer = window.setTimeout(() => {
            setSearch({ query: trimmed, status: "loading" });
            searchBrochurePrograms({ q: trimmed, limit: 20, signal: controller.signal }).then(
                ({ data }) => setSearch({ query: trimmed, status: "ready", results: data }),
                (cause: unknown) => {
                    if (!controller.signal.aborted)
                        setSearch({ query: trimmed, status: "error", error: cause });
                }
            );
        }, SEARCH_DEBOUNCE_MS);
        return () => {
            window.clearTimeout(timer);
            controller.abort();
        };
    }, [trimmed]);

    const applied = new Set(existing.map((item) => item.program_identifier));
    const isSelected = (identifier: string) =>
        selected.some((item) => item.program_identifier === identifier);
    const full = selected.length >= APPLICATION_BATCH_LIMIT;
    const visibleSearch = trimmed && search?.query === trimmed ? search : null;

    const submit = () =>
        run(async () => {
            const { data } = await createApplications(
                selected.map((item) => item.program_identifier)
            );
            onCreated(data);
            setSelected([]);
            setQuery("");
        });

    return (
        <Card
            title="新增申請校系"
            description={`搜尋並勾選你申請的校系，一次最多 ${APPLICATION_BATCH_LIMIT} 個。`}
            className="flex flex-col gap-5"
        >
            <Field label="搜尋校系">
                <div className="relative">
                    <Search
                        aria-hidden
                        className="pointer-events-none absolute top-1/2 left-4 size-5 -translate-y-1/2 text-copy-muted"
                    />
                    <Input
                        type="search"
                        value={query}
                        placeholder="輸入學校或學系名稱"
                        onChange={(event) => setQuery(event.target.value)}
                        className="pl-12"
                    />
                </div>
            </Field>

            <StatusText>{visibleSearch?.status === "loading" && "搜尋中…"}</StatusText>
            <ErrorText>
                {visibleSearch?.status === "error" && describeError(visibleSearch.error)}
            </ErrorText>
            {visibleSearch?.status === "ready" &&
                (visibleSearch.results.length === 0 ? (
                    <p className="font-sans text-base text-copy-muted">找不到符合的校系。</p>
                ) : (
                    <ul
                        aria-label="搜尋結果"
                        className="flex max-h-80 flex-col divide-y divide-ink/10 overflow-y-auto rounded-[var(--radius-control)] border border-ink/10"
                    >
                        {visibleSearch.results.map((program) => {
                            const alreadyApplied = applied.has(program.program_identifier);
                            const picked = isSelected(program.program_identifier);
                            return (
                                <li
                                    key={program.program_identifier}
                                    className="flex items-center gap-3 px-4 py-3"
                                >
                                    <span className="min-w-0 flex-1 font-sans text-base break-words text-ink">
                                        {programLabel(program)}
                                    </span>
                                    {alreadyApplied ? (
                                        <span className="shrink-0 font-sans text-sm text-copy-muted">
                                            已登記
                                        </span>
                                    ) : (
                                        <Button
                                            type="button"
                                            variant="secondary"
                                            size="sm"
                                            className="shrink-0 gap-1 px-3"
                                            disabled={picked || full}
                                            onClick={() =>
                                                setSelected((items) => [...items, program])
                                            }
                                        >
                                            <Plus aria-hidden className="size-4" />
                                            {picked ? "已加入" : "加入"}
                                        </Button>
                                    )}
                                </li>
                            );
                        })}
                    </ul>
                ))}

            {selected.length > 0 && (
                <section aria-labelledby="selected-programs-title" className="flex flex-col gap-3">
                    <h3
                        id="selected-programs-title"
                        className="font-sans text-base font-medium text-ink"
                    >
                        已選擇 {selected.length} / {APPLICATION_BATCH_LIMIT}
                    </h3>
                    <ul className="flex flex-col gap-2">
                        {selected.map((program) => (
                            <li
                                key={program.program_identifier}
                                className="flex items-center gap-2 rounded-[var(--radius-control)] bg-accent-green/40 py-1 pr-1 pl-4"
                            >
                                <span className="min-w-0 flex-1 font-sans text-base break-words text-ink">
                                    {programLabel(program)}
                                </span>
                                <IconButton
                                    type="button"
                                    label={`移除 ${programLabel(program)}`}
                                    icon={<X className="size-4" />}
                                    className="size-10 shrink-0 hover:bg-ink/5"
                                    onClick={() =>
                                        setSelected((items) =>
                                            items.filter(
                                                (item) =>
                                                    item.program_identifier !==
                                                    program.program_identifier
                                            )
                                        )
                                    }
                                />
                            </li>
                        ))}
                    </ul>
                    <p className="flex items-start gap-2 font-sans text-sm text-copy-muted">
                        <TriangleAlert
                            aria-hidden
                            className="mt-0.5 size-4 shrink-0 text-accent-yellow-strong"
                        />
                        送出後申請校系會立即鎖定，之後無法自行修改或刪除；如有錯誤需聯絡客服處理。
                    </p>
                    <div className="flex flex-wrap items-center gap-4">
                        <ConfirmDialog
                            trigger={
                                <Button disabled={submitting} size="base">
                                    {submitting ? "送出中…" : `送出 ${selected.length} 個校系`}
                                </Button>
                            }
                            title="確認送出申請校系？"
                            description={`將登記 ${selected.length} 個校系。送出後會立即鎖定，無法自行修改或刪除，請再次確認選擇正確。`}
                            confirmLabel="確認送出"
                            tone="primary"
                            onConfirm={submit}
                        />
                        <ErrorText>{error}</ErrorText>
                    </div>
                </section>
            )}
        </Card>
    );
}
