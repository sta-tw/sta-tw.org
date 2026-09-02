"use client";

import { Download, ExternalLink, Heart } from "lucide-react";
import { Tabs } from "radix-ui";
import type { Brochure, BrochureFact } from "../data/brochures";
import type { OpenGraphPreview } from "../lib/open-graph";

type BrochureDetailTabsProps = {
    brochure: Brochure;
    externalLinkPreviews: OpenGraphPreview[];
};

const tabs = [
    { value: "overview", label: "招生資訊" },
    { value: "about", label: "關於學系" },
    { value: "registration", label: "報名資訊" },
    { value: "history", label: "歷年招生資料" }
];

export default function BrochureDetailTabs({
    brochure,
    externalLinkPreviews
}: BrochureDetailTabsProps) {
    return (
        <Tabs.Root defaultValue="overview" className="mt-7 sm:mt-9">
            <div className="-mx-5 overflow-x-auto px-5 pb-2 sm:mx-0 sm:px-0">
                <Tabs.List
                    aria-label="簡章內容"
                    className="flex w-max min-w-full gap-2 border-b border-ink/15 pb-3 sm:gap-3"
                >
                    {tabs.map((tab) => (
                        <Tabs.Trigger
                            key={tab.value}
                            value={tab.value}
                            className="cursor-pointer rounded-[var(--radius-small)] bg-accent-green/45 px-3 py-2 font-sans text-sm font-medium whitespace-nowrap text-ink transition-colors hover:bg-accent-green/80 focus-visible:ring-2 focus-visible:ring-ink focus-visible:outline-none data-[state=active]:bg-accent-green-strong sm:text-base"
                        >
                            {tab.label}
                        </Tabs.Trigger>
                    ))}
                </Tabs.List>
            </div>

            <div className="mt-4 flex flex-col gap-3 sm:mt-5 sm:flex-row sm:flex-wrap sm:items-center">
                <p className="w-fit rounded-[var(--radius-small)] bg-accent-yellow px-4 py-2 font-sans text-sm leading-snug text-ink sm:text-base">
                    此系設有 A、B 兩組，返回上一頁以查看不同組別細則
                </p>
                <div className="flex flex-wrap gap-2">
                    <button
                        type="button"
                        className="inline-flex h-10 cursor-pointer items-center gap-2 rounded-full bg-accent-yellow/75 px-3 font-sans text-sm font-medium text-ink transition-colors hover:bg-accent-yellow focus-visible:ring-2 focus-visible:ring-ink focus-visible:ring-offset-2 focus-visible:outline-none"
                    >
                        <Download aria-hidden className="h-4 w-4" />
                        簡章下載
                    </button>
                    <button
                        type="button"
                        className="inline-flex h-10 cursor-pointer items-center gap-2 rounded-full bg-accent-yellow/75 px-3 font-sans text-sm font-medium text-ink transition-colors hover:bg-accent-yellow focus-visible:ring-2 focus-visible:ring-ink focus-visible:ring-offset-2 focus-visible:outline-none"
                    >
                        <Heart aria-hidden className="h-4 w-4" />
                        我對此學系有興趣
                    </button>
                </div>
            </div>

            <Tabs.Content value="overview" className="outline-none">
                <OverviewTab brochure={brochure} />
            </Tabs.Content>
            <Tabs.Content value="about" className="outline-none">
                <ExternalLinksTab previews={externalLinkPreviews} />
            </Tabs.Content>
            <Tabs.Content value="registration" className="outline-none">
                <RegistrationTab items={brochure.registration} />
            </Tabs.Content>
            <Tabs.Content value="history" className="outline-none">
                <HistoryTab brochure={brochure} />
            </Tabs.Content>
        </Tabs.Root>
    );
}

function OverviewTab({ brochure }: Pick<BrochureDetailTabsProps, "brochure">) {
    return (
        <div className="mt-8 space-y-7 sm:mt-10 sm:space-y-8">
            <dl className="flex flex-wrap gap-x-8 gap-y-4 sm:gap-x-10">
                {brochure.facts.map((fact) => (
                    <Fact key={fact.label} {...fact} compact />
                ))}
            </dl>
            <dl className="space-y-5 sm:space-y-6">
                <Fact label="報名資格" value={brochure.eligibility} />
                <Fact label="考試方式" value={brochure.examFormat} />
                <Fact label="報名費用" value={brochure.fee} />
                <Fact label="報名時程" value={brochure.timeline} />
            </dl>
        </div>
    );
}

function ExternalLinksTab({ previews }: { previews: OpenGraphPreview[] }) {
    if (previews.length === 0) {
        return (
            <div className="mt-8 rounded-[var(--radius-panel)] border border-dashed border-ink/20 bg-surface/55 px-5 py-10 text-center sm:mt-10">
                <p className="font-sans text-base text-ink/65">目前沒有可用的相關連結。</p>
            </div>
        );
    }

    return (
        <ul className="mt-8 space-y-4 sm:mt-10 sm:space-y-5">
            {previews.map((preview) => (
                <li key={preview.url}>
                    <a
                        href={preview.url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="group grid overflow-hidden rounded-[var(--radius-panel)] border border-ink/10 bg-surface/80 transition-[border-color,box-shadow,transform] hover:-translate-y-0.5 hover:border-ink/25 hover:shadow-[var(--shadow-card)] focus-visible:ring-2 focus-visible:ring-ink focus-visible:ring-offset-2 focus-visible:outline-none sm:grid-cols-[14rem_minmax(0,1fr)]"
                    >
                        <div
                            aria-hidden
                            className="min-h-40 bg-accent-green/60 bg-cover bg-center sm:min-h-full"
                            style={
                                preview.imageUrl
                                    ? { backgroundImage: `url(\"${preview.imageUrl}\")` }
                                    : undefined
                            }
                        />
                        <div className="min-w-0 p-5 sm:p-6">
                            <p className="font-sans text-sm text-ink/55">{preview.siteName}</p>
                            <h2 className="mt-2 flex items-start gap-2 font-sans text-xl leading-snug font-medium text-ink sm:text-2xl">
                                <span>{preview.title}</span>
                                <ExternalLink
                                    aria-hidden
                                    className="mt-1 h-4 w-4 shrink-0 text-ink/55"
                                />
                            </h2>
                            {preview.description && (
                                <p className="mt-4 line-clamp-3 font-sans text-base leading-7 text-ink/70">
                                    {preview.description}
                                </p>
                            )}
                        </div>
                    </a>
                </li>
            ))}
        </ul>
    );
}

function RegistrationTab({ items }: { items: BrochureFact[] }) {
    return (
        <div className="mt-8 max-w-2xl overflow-x-auto rounded-[var(--radius-small)] border border-ink/15 bg-surface/65 sm:mt-10">
            <dl className="min-w-125 divide-y divide-ink/15">
                {items.map((item) => (
                    <div key={item.label} className="grid grid-cols-[11rem_minmax(0,1fr)]">
                        <dt className="border-r border-ink/15 bg-ink/3 px-4 py-3 font-sans font-medium text-ink">
                            {item.label}
                        </dt>
                        <dd className="px-4 py-3 font-sans text-ink/75">{item.value}</dd>
                    </div>
                ))}
            </dl>
        </div>
    );
}

function HistoryTab({ brochure }: Pick<BrochureDetailTabsProps, "brochure">) {
    return (
        <div className="mt-8 overflow-x-auto rounded-[var(--radius-small)] border border-ink/15 bg-surface/65 sm:mt-10">
            <table className="w-full min-w-180 border-collapse text-center font-sans text-sm sm:text-base">
                <caption className="sr-only">歷年招生資料</caption>
                <thead className="bg-ink/3 font-medium text-ink">
                    <tr className="divide-x divide-ink/15">
                        <th scope="col" className="px-4 py-3 font-medium">
                            年份
                        </th>
                        <th scope="col" className="px-4 py-3 font-medium">
                            正取人數
                        </th>
                        <th scope="col" className="px-4 py-3 font-medium">
                            遞補人數
                        </th>
                        <th scope="col" className="px-4 py-3 font-medium">
                            候補人數
                        </th>
                        <th scope="col" className="px-4 py-3 font-medium">
                            報名人數
                        </th>
                    </tr>
                </thead>
                <tbody className="divide-y divide-ink/15 text-ink/75">
                    {brochure.history.map((row) => (
                        <tr key={row.year} className="divide-x divide-ink/15">
                            <td className="px-4 py-3">{row.year}</td>
                            <td className="px-4 py-3">{row.admitted}</td>
                            <td className="px-4 py-3">{row.waitlisted}</td>
                            <td className="px-4 py-3">{row.candidates}</td>
                            <td className="px-4 py-3">{row.applicants}</td>
                        </tr>
                    ))}
                </tbody>
            </table>
        </div>
    );
}

function Fact({ label, value, compact = false }: BrochureFact & { compact?: boolean }) {
    return (
        <div
            className={
                compact
                    ? "flex items-center gap-3"
                    : "flex flex-col gap-2 sm:flex-row sm:items-start sm:gap-4"
            }
        >
            <dt className="w-fit shrink-0 rounded-[var(--radius-small)] bg-accent-green/55 px-2 py-1 font-sans text-base font-medium text-ink">
                {label}
            </dt>
            <dd className="font-sans text-base leading-7 text-ink/85">{value}</dd>
        </div>
    );
}
