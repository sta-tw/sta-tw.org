"use client";

import Link from "next/link";
import { useState } from "react";
import { Search as SearchIcon } from "lucide-react";
import Button from "../components/button";
import { search, type SearchResults, type SearchType } from "../lib/api/search";
import { ApiError } from "../lib/api/types";

const typeLabels: Record<SearchType, string> = {
    schools: "學校",
    programs: "校系管道",
    experiences: "心得文章"
};

const inputClass =
    "w-full rounded-[var(--radius-small)] border border-ink/15 bg-surface px-4 py-3 font-sans text-base text-ink outline-none transition-colors placeholder:text-copy-muted focus:border-ink/40";

function describeError(cause: unknown): string {
    if (cause instanceof ApiError) {
        if (cause.code === "rate_limited") return "搜尋太頻繁了，請稍等一下再試。";
        return cause.message || `發生錯誤（${cause.code}）`;
    }
    return "發生未知錯誤，請稍後再試。";
}

export default function SearchView() {
    const [query, setQuery] = useState("");
    const [results, setResults] = useState<SearchResults | null>(null);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [searchedFor, setSearchedFor] = useState<string | null>(null);

    async function handleSubmit(event: React.FormEvent) {
        event.preventDefault();
        const q = query.trim();
        if (!q) return;
        setLoading(true);
        setError(null);
        try {
            const response = await search(q);
            setResults(response.results);
            setSearchedFor(response.query);
        } catch (cause) {
            setError(describeError(cause));
            setResults(null);
        } finally {
            setLoading(false);
        }
    }

    const totalHits =
        (results?.schools?.length ?? 0) +
        (results?.programs?.length ?? 0) +
        (results?.experiences?.length ?? 0);

    return (
        <div className="flex flex-col gap-8">
            <div>
                <h1 className="font-serif text-hero-subtitle text-ink">搜尋</h1>
                <p className="mt-2 font-sans text-copy-muted">搜尋學校、校系管道與心得文章。</p>
            </div>

            <form onSubmit={handleSubmit} className="flex gap-3">
                <input
                    className={inputClass}
                    placeholder="輸入關鍵字，例如：清華 / 資工 / 面試"
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    maxLength={200}
                />
                <Button
                    type="submit"
                    disabled={loading || !query.trim()}
                    className="shrink-0 gap-2 px-6"
                >
                    <SearchIcon aria-hidden className="h-5 w-5" />
                    {loading ? "搜尋中…" : "搜尋"}
                </Button>
            </form>

            {error ? <p className="font-sans text-sm text-red-600">{error}</p> : null}

            {searchedFor !== null && !error ? (
                <p className="font-sans text-sm text-copy-muted">
                    「{searchedFor}」共 {totalHits} 筆結果
                </p>
            ) : null}

            {results ? (
                <div className="flex flex-col gap-8">
                    <ResultSection title={typeLabels.schools}>
                        {results.schools?.map((hit) => (
                            <ResultCard
                                key={hit.id}
                                title={hit.school_name}
                                subtitle={hit.institution_type}
                            />
                        ))}
                    </ResultSection>
                    <ResultSection title={typeLabels.programs}>
                        {results.programs?.map((hit) => (
                            <ResultCard
                                key={hit.id}
                                title={`${hit.school_name} · ${hit.admission_program_name}`}
                                href={`/bochures/program?identifier=${encodeURIComponent(hit.program_identifier)}`}
                                subtitle={[
                                    hit.academic_year && `${hit.academic_year} 學年度`,
                                    hit.special_talent_target
                                ]
                                    .filter(Boolean)
                                    .join(" · ")}
                            />
                        ))}
                    </ResultSection>
                    <ResultSection title={typeLabels.experiences}>
                        {results.experiences?.map((hit) => (
                            <ResultCard
                                key={hit.id}
                                title={hit.title}
                                subtitle={hit.snippet}
                                href={`/article/experience?id=${encodeURIComponent(hit.id)}`}
                            />
                        ))}
                    </ResultSection>
                </div>
            ) : null}
        </div>
    );
}

function ResultSection({ title, children }: { title: string; children: React.ReactNode }) {
    const items = Array.isArray(children) ? children.filter(Boolean) : children ? [children] : [];
    if (items.length === 0) return null;
    return (
        <div className="flex flex-col gap-3">
            <h2 className="font-serif text-xl text-ink">{title}</h2>
            <div className="flex flex-col gap-2">{items}</div>
        </div>
    );
}

function ResultCard({
    title,
    subtitle,
    href
}: {
    title: string;
    subtitle?: string;
    href?: string;
}) {
    const content = (
        <>
            <p className="font-sans text-ink">{title}</p>
            {subtitle ? <p className="mt-1 font-sans text-sm text-copy-muted">{subtitle}</p> : null}
        </>
    );

    if (!href) {
        return (
            <div className="rounded-[var(--radius-small)] border border-ink/10 bg-surface px-5 py-4">
                {content}
            </div>
        );
    }

    return (
        <Link
            href={href}
            className="rounded-[var(--radius-small)] border border-ink/10 bg-surface px-5 py-4 transition-colors hover:border-ink/25 hover:bg-ink/5 focus-visible:ring-2 focus-visible:ring-ink focus-visible:ring-offset-2 focus-visible:outline-none"
        >
            {content}
        </Link>
    );
}
