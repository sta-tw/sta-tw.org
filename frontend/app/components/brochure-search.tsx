import { ChevronDown, Search } from "lucide-react";
import { brochureFilters } from "../data/brochure-filters";
import InfoTooltip from "./info-tooltip";

const LAST_UPDATED = "2026 / 9 / 01";

export default function BrochureSearch() {
    return (
        <main className="article-dots flex flex-1 border-y border-ink/5">
            <section className="mx-auto flex w-full max-w-screen-xl flex-col px-5 py-12 sm:px-6 sm:py-16 lg:min-h-[42rem] lg:px-16 lg:py-16">
                <div className="flex flex-col items-center gap-4 border-b border-ink/10 pb-8 sm:gap-5 lg:relative lg:min-h-48 lg:justify-center lg:pb-10">
                    <h1 className="font-serif text-4xl tracking-[-0.04em] text-ink sm:text-5xl lg:text-6xl">
                        簡章搜尋
                    </h1>
                    <time
                        dateTime="2026-09-01"
                        className="font-sans text-sm text-ink/60 sm:text-base lg:absolute lg:right-0 lg:bottom-10"
                    >
                        資料更新：{LAST_UPDATED}
                    </time>
                </div>

                <form action="/bochures" className="mt-7 sm:mt-9">
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
                            placeholder="試著搜尋「淡江大學洗車學系」"
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
                                            defaultValue=""
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
            </section>
        </main>
    );
}
