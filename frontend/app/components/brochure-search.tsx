"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { ArrowRight, ChevronDown, Search } from "lucide-react";
import { brochureFilters } from "../data/brochure-filters";
import type { BrochureSearchFilters } from "../lib/brochure-types";
import { searchBrochurePrograms, type BrochureSearchProgram } from "../lib/api/admissions";
import { ApiError } from "../lib/api/types";
import { publicPath } from "../lib/public-path";
import InfoTooltip from "./info-tooltip";

const DATA_SOURCE_LABEL = "日曆小幫手";
const RESULTS_PER_PAGE = 50;

type BrochureSearchState = {
    key: string;
    results: BrochureSearchProgram[];
    total: number;
    error: string | null;
};

function hasDisplayValue(value: string) {
    return Boolean(value.trim() && value.trim() !== "-");
}

export default function BrochureSearch() {
    const searchParams = useSearchParams();
    const filters: BrochureSearchFilters = {};
    const queryParam = searchParams.get("q")?.trim();
    if (queryParam) filters.q = queryParam;
    for (const filter of brochureFilters) {
        const value = searchParams.get(filter.id);
        if (value === "required" || value === "not-required") {
            filters[filter.id] = value;
        }
    }

    const query = filters.q?.trim() || undefined;
    const skillsTestFilter = filters["skills-test"];
    const interviewFilter = filters.interview;
    const portfolioFilter = filters.portfolio;
    const rawPage = Number(searchParams.get("page") ?? "1");
    const maxPage = Math.floor(10_000 / RESULTS_PER_PAGE) + 1;
    const page = Number.isInteger(rawPage) && rawPage > 0 ? Math.min(rawPage, maxPage) : 1;
    const requestKey = [
        query ?? "",
        skillsTestFilter ?? "",
        interviewFilter ?? "",
        portfolioFilter ?? "",
        page
    ].join("\u001f");
    const [state, setState] = useState<BrochureSearchState>({
        key: "",
        results: [],
        total: 0,
        error: null
    });
    const isLoading = state.key !== requestKey;
    const results = state.key === requestKey ? state.results : [];
    const total = state.key === requestKey ? state.total : 0;
    const error = state.key === requestKey ? state.error : null;
    const hasFilters = Boolean(filters.q || brochureFilters.some((filter) => filters[filter.id]));
    const pageCount = Math.max(1, Math.ceil(total / RESULTS_PER_PAGE));

    function pageHref(targetPage: number) {
        const params = new URLSearchParams(searchParams.toString());
        if (targetPage <= 1) params.delete("page");
        else params.set("page", String(targetPage));
        const queryString = params.toString();
        return publicPath(`/bochures${queryString ? `?${queryString}` : ""}`);
    }

    useEffect(() => {
        const controller = new AbortController();

        searchBrochurePrograms({
            q: query,
            skillsTest: skillsTestFilter,
            interview: interviewFilter,
            portfolio: portfolioFilter,
            limit: RESULTS_PER_PAGE,
            offset: (page - 1) * RESULTS_PER_PAGE,
            signal: controller.signal
        })
            .then((response) => {
                if (controller.signal.aborted) return;
                setState({
                    key: requestKey,
                    results: response.data,
                    total: response.meta?.count ?? response.data.length,
                    error: null
                });
            })
            .catch((cause: unknown) => {
                if (controller.signal.aborted) return;
                setState({
                    key: requestKey,
                    results: [],
                    total: 0,
                    error:
                        cause instanceof ApiError
                            ? cause.message
                            : "目前無法取得後端簡章資料，請稍後再試。"
                });
            });

        return () => controller.abort();
    }, [requestKey, query, skillsTestFilter, interviewFilter, portfolioFilter, page]);

    return (
        <main className="article-dots flex flex-1 border-y border-ink/5">
            <section className="mx-auto flex w-full max-w-screen-xl flex-col px-5 py-12 sm:px-6 sm:py-16 lg:min-h-[42rem] lg:px-16 lg:py-16">
                <div className="flex flex-col items-center gap-4 border-b border-ink/10 pb-8 sm:gap-5 lg:relative lg:min-h-48 lg:justify-center lg:pb-10">
                    <h1 className="font-serif text-4xl tracking-[-0.04em] text-ink sm:text-5xl lg:text-6xl">
                        簡章搜尋
                    </h1>
                    <Link
                        href={publicPath("/report-data")}
                        className="font-sans text-sm font-bold text-ink underline decoration-ink/30 underline-offset-4 transition-colors hover:decoration-ink sm:text-base"
                    >
                        提供或補充招生資料
                    </Link>
                    <p className="font-sans text-sm text-ink/60 sm:text-base lg:absolute lg:right-0 lg:bottom-10">
                        資料來源：{DATA_SOURCE_LABEL}
                    </p>
                </div>

                <form action={publicPath("/bochures")} className="mt-7 sm:mt-9">
                    <label htmlFor="brochure-keyword" className="sr-only">
                        搜尋簡章
                    </label>
                    <div className="group flex min-h-15 items-center gap-3 rounded-[var(--radius-control)] border-2 border-[#f6bd42] bg-accent-yellow/80 px-4 shadow-sm transition-shadow focus-within:shadow-[0_0_0_4px_rgba(255,225,132,0.45)] sm:min-h-17 sm:px-5">
                        <Search aria-hidden className="h-5 w-5 shrink-0 text-ink sm:h-6 sm:w-6" />
                        <input
                            id="brochure-keyword"
                            name="q"
                            type="search"
                            autoComplete="off"
                            defaultValue={filters.q}
                            placeholder="試著搜尋「國立複雜大學 超級複雜系」"
                            className="min-w-0 flex-1 bg-transparent font-sans text-base text-ink placeholder:text-ink/50 focus:outline-none sm:text-xl"
                        />
                        <button
                            type="submit"
                            className="hidden h-10 shrink-0 cursor-pointer rounded-[calc(var(--radius-control)-0.25rem)] bg-button px-4 font-sans text-sm font-bold text-button-foreground transition-colors hover:bg-button-hover focus-visible:ring-2 focus-visible:ring-ink focus-visible:ring-offset-2 focus-visible:outline-none sm:inline-flex sm:items-center"
                        >
                            搜尋
                        </button>
                    </div>

                    <fieldset className="mt-9 border-0 p-0 sm:mt-11">
                        <legend className="flex items-center gap-1.5 font-serif text-2xl text-ink sm:text-3xl">
                            篩選器
                            <span className="font-sans text-base text-ink/55 sm:text-lg">
                                （可自由選填）
                            </span>
                            <InfoTooltip label="篩選器說明">
                                選擇一項或多項條件，縮小符合需求的校系與招生簡章範圍。
                            </InfoTooltip>
                        </legend>

                        <div className="mt-6 grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-3 lg:gap-7">
                            {brochureFilters.map((filter) => (
                                <div key={filter.id} className="min-w-0">
                                    <label
                                        htmlFor={filter.id}
                                        className="mb-2 block font-serif text-lg text-ink sm:text-xl"
                                    >
                                        {filter.label}
                                    </label>
                                    <div className="relative">
                                        <select
                                            id={filter.id}
                                            name={filter.id}
                                            defaultValue={filters[filter.id] ?? ""}
                                            className="h-13 w-full cursor-pointer appearance-none rounded-[var(--radius-small)] border border-ink/15 bg-surface px-4 pr-11 font-sans text-base text-ink shadow-sm transition-colors hover:border-ink/30 focus:border-ink focus:ring-2 focus:ring-ink/15 focus:outline-none"
                                        >
                                            <option value="">請選擇</option>
                                            {filter.options.map((option) => (
                                                <option key={option.value} value={option.value}>
                                                    {option.label}
                                                </option>
                                            ))}
                                        </select>
                                        <ChevronDown
                                            aria-hidden
                                            className="pointer-events-none absolute top-1/2 right-4 h-5 w-5 -translate-y-1/2 text-ink/70"
                                        />
                                    </div>
                                </div>
                            ))}
                        </div>
                    </fieldset>
                </form>

                <section
                    aria-labelledby="brochure-results"
                    className="mt-10 border-t border-ink/10 pt-8 sm:mt-12 sm:pt-10"
                >
                    <div className="flex flex-wrap items-baseline justify-between gap-3">
                        <div>
                            <h2
                                id="brochure-results"
                                className="font-serif text-2xl text-ink sm:text-3xl"
                            >
                                搜尋結果
                            </h2>
                            <p
                                aria-live="polite"
                                className="mt-2 font-sans text-sm text-ink/60 sm:text-base"
                            >
                                {isLoading
                                    ? "正在載入後端簡章資料…"
                                    : error
                                      ? "簡章資料載入失敗"
                                      : `找到 ${total} 筆符合條件的簡章`}
                            </p>
                        </div>
                        {hasFilters && (
                            <Link
                                href={publicPath("/bochures")}
                                prefetch={false}
                                className="font-sans text-sm text-ink/65 underline decoration-ink/35 underline-offset-4 transition-colors hover:text-ink sm:text-base"
                            >
                                清除篩選條件
                            </Link>
                        )}
                    </div>

                    {error ? (
                        <div
                            role="alert"
                            className="mt-5 rounded-[var(--radius-panel)] border border-dashed border-ink/20 bg-surface/55 px-5 py-9 text-center sm:mt-6 sm:py-12"
                        >
                            <p className="font-sans text-lg font-medium text-ink">
                                無法載入簡章資料
                            </p>
                            <p className="mt-2 font-sans text-sm leading-relaxed text-ink/60 sm:text-base">
                                {error}
                            </p>
                        </div>
                    ) : isLoading ? (
                        <div className="mt-5 rounded-[var(--radius-panel)] border border-ink/10 bg-surface/55 px-5 py-9 text-center sm:mt-6 sm:py-12">
                            <p className="font-sans text-lg text-ink/70">正在從後端取得資料…</p>
                        </div>
                    ) : results.length > 0 ? (
                        <ul className="mt-5 grid gap-4 sm:mt-6 sm:grid-cols-2">
                            {results.map((program) => {
                                const match =
                                    program.admission_program_name.match(
                                        /[（(]([^（）()]+)[）)]\s*$/
                                    );
                                const department = match
                                    ? program.admission_program_name.slice(0, match.index).trim()
                                    : program.admission_program_name;
                                const group = match?.[1].trim() ?? "";
                                const facts = [
                                    { label: "學年度", value: `${program.academic_year} 學年度` },
                                    ...(hasDisplayValue(program.admission_group)
                                        ? [
                                              {
                                                  label: "學群",
                                                  value: hasDisplayValue(program.cross_group)
                                                      ? `${program.admission_group} 跨 ${program.cross_group}`
                                                      : program.admission_group
                                              }
                                          ]
                                        : []),
                                    ...(hasDisplayValue(program.admission_category)
                                        ? [{ label: "學類", value: program.admission_category }]
                                        : []),
                                    { label: "招生人數", value: `${program.admission_quota} 人` }
                                ];

                                return (
                                    <li key={program.program_identifier}>
                                        <Link
                                            href={`/bochures/program/${encodeURIComponent(program.program_identifier)}/`}
                                            prefetch={false}
                                            className="group flex h-full flex-col rounded-[var(--radius-panel)] border border-ink/10 bg-surface/80 p-5 shadow-sm transition-[border-color,box-shadow,transform] hover:-translate-y-0.5 hover:border-ink/25 hover:shadow-[var(--shadow-card)] focus-visible:ring-2 focus-visible:ring-ink focus-visible:ring-offset-2 focus-visible:outline-none sm:p-6"
                                        >
                                            <p className="font-sans text-sm text-ink/60">
                                                {program.school_name}
                                            </p>
                                            <h3 className="mt-2 font-sans text-xl leading-snug font-medium text-ink sm:text-2xl">
                                                {department}
                                                {group ? `（${group}）` : ""}
                                            </h3>
                                            <dl className="mt-5 flex flex-wrap gap-x-4 gap-y-2">
                                                {facts.map((fact) => (
                                                    <div
                                                        key={fact.label}
                                                        className="flex items-center gap-1.5"
                                                    >
                                                        <dt className="rounded-md bg-accent-green/55 px-1.5 py-0.5 font-sans text-xs font-medium text-ink">
                                                            {fact.label}
                                                        </dt>
                                                        <dd className="font-sans text-sm text-ink/70">
                                                            {fact.value}
                                                        </dd>
                                                    </div>
                                                ))}
                                            </dl>
                                            <span className="mt-6 inline-flex items-center gap-1.5 font-sans text-sm font-medium text-ink transition-transform group-hover:translate-x-1">
                                                查看簡章{" "}
                                                <ArrowRight aria-hidden className="h-4 w-4" />
                                            </span>
                                        </Link>
                                    </li>
                                );
                            })}
                        </ul>
                    ) : (
                        <div className="mt-5 rounded-[var(--radius-panel)] border border-dashed border-ink/20 bg-surface/55 px-5 py-9 text-center sm:mt-6 sm:py-12">
                            <p className="font-sans text-lg font-medium text-ink">
                                暫時沒有符合條件的簡章
                            </p>
                            <p className="mt-2 font-sans text-sm leading-relaxed text-ink/60 sm:text-base">
                                試著調整關鍵字或移除部分篩選條件。
                            </p>
                        </div>
                    )}

                    {!isLoading && !error && pageCount > 1 && (
                        <nav
                            aria-label="簡章搜尋分頁"
                            className="mt-8 flex items-center justify-center gap-4 font-sans text-sm text-ink/70"
                        >
                            {page > 1 ? (
                                <Link
                                    href={pageHref(page - 1)}
                                    prefetch={false}
                                    className="rounded-md border border-ink/15 px-4 py-2 underline decoration-ink/30 underline-offset-4 hover:text-ink"
                                >
                                    上一頁
                                </Link>
                            ) : (
                                <span
                                    aria-disabled="true"
                                    className="rounded-md border border-ink/10 px-4 py-2 text-ink/35"
                                >
                                    上一頁
                                </span>
                            )}
                            <span>
                                第 {page} / {pageCount} 頁
                            </span>
                            {page < pageCount ? (
                                <Link
                                    href={pageHref(page + 1)}
                                    prefetch={false}
                                    className="rounded-md border border-ink/15 px-4 py-2 underline decoration-ink/30 underline-offset-4 hover:text-ink"
                                >
                                    下一頁
                                </Link>
                            ) : (
                                <span
                                    aria-disabled="true"
                                    className="rounded-md border border-ink/10 px-4 py-2 text-ink/35"
                                >
                                    下一頁
                                </span>
                            )}
                        </nav>
                    )}
                </section>
            </section>
        </main>
    );
}
