"use client";

import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { LogIn, Search } from "lucide-react";
import { useCallback } from "react";
import { VerificationRequired, isVerificationRequired } from "../account/verification-required";
import Button from "../components/button";
import { Card } from "../components/card";
import PortfolioCard from "../components/portfolio-card";
import { EmptyState, Resource } from "../components/status";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "../components/tabs";
import { browsePortfolios, portfolioUpdatedAt } from "../lib/api/portfolio";
import { ApiError } from "../lib/api/types";
import { formatNumericDate } from "../lib/format";
import { loginHref, portfolioEditorHref } from "../lib/routes";
import { useResource } from "../lib/use-resource";

type Tab = "public" | "exchange";

const tabs: { value: Tab; label: string; description: string }[] = [
    {
        value: "public",
        label: "公開備審",
        description: "學長姐公開分享的備審，所有人都能瀏覽與下載。"
    },
    {
        value: "exchange",
        label: "交換備審",
        description: "僅在已驗證的特選生之間交換，分享自己的備審也能看到別人的。"
    }
];

export default function PortfolioBrowser() {
    const router = useRouter();
    const pathname = usePathname();
    const searchParams = useSearchParams();
    const q = searchParams.get("q")?.trim() ?? "";
    const tab: Tab = searchParams.get("type") === "exchange" ? "exchange" : "public";

    const navigate = (next: { q?: string; type?: Tab }) => {
        const params = new URLSearchParams();
        const nextQ = next.q ?? q;
        const nextType = next.type ?? tab;
        if (nextQ) params.set("q", nextQ);
        if (nextType !== "public") params.set("type", nextType);
        const query = params.toString();
        router.replace(query ? `${pathname}?${query}` : pathname, { scroll: false });
    };

    return (
        <main className="article-dots flex-1">
            <div className="mx-auto flex w-full max-w-screen-lg flex-col px-5 py-12 sm:px-6 sm:py-16 lg:px-8">
                <header className="flex flex-col items-center gap-3 border-b border-ink/10 pb-8 text-center sm:pb-10">
                    <h1 className="font-serif text-4xl tracking-[-0.04em] text-ink sm:text-5xl">
                        交換與公開備審
                    </h1>
                    <p className="max-w-xl font-sans text-base text-copy-muted">
                        參考學長姐準備的備審資料，找到適合自己的呈現方式。
                    </p>
                    <Link
                        href={portfolioEditorHref()}
                        className="font-sans text-sm font-bold text-ink underline decoration-ink/30 underline-offset-4 transition-colors hover:decoration-ink sm:text-base"
                    >
                        分享我的備審
                    </Link>
                </header>

                <form
                    role="search"
                    className="mt-7 sm:mt-9"
                    onSubmit={(event) => {
                        event.preventDefault();
                        const value = new FormData(event.currentTarget).get("q");
                        navigate({ q: typeof value === "string" ? value.trim() : "" });
                    }}
                >
                    <label htmlFor="portfolio-keyword" className="sr-only">
                        搜尋備審
                    </label>
                    <div className="flex min-h-15 items-center gap-3 rounded-[var(--radius-control)] border-2 border-accent-yellow-deep bg-accent-yellow/80 px-4 shadow-sm transition-shadow focus-within:shadow-[0_0_0_4px_rgba(255,225,132,0.45)] sm:min-h-17 sm:px-5">
                        <Search aria-hidden className="size-5 shrink-0 text-ink sm:size-6" />
                        <input
                            key={q}
                            id="portfolio-keyword"
                            name="q"
                            type="search"
                            autoComplete="off"
                            defaultValue={q}
                            placeholder="試著搜尋「資訊工程學系」"
                            className="min-w-0 flex-1 bg-transparent font-sans text-base text-ink placeholder:text-ink/50 focus:outline-none sm:text-xl"
                        />
                        <button
                            type="submit"
                            className="hidden h-10 shrink-0 cursor-pointer rounded-[calc(var(--radius-control)-0.25rem)] bg-button px-4 font-sans text-sm font-bold text-button-foreground focus-ring transition-colors hover:bg-button-hover sm:inline-flex sm:items-center"
                        >
                            搜尋
                        </button>
                    </div>
                </form>

                <Tabs
                    value={tab}
                    onValueChange={(value) => navigate({ type: value as Tab })}
                    className="mt-8 sm:mt-10"
                >
                    <TabsList aria-label="備審類型">
                        {tabs.map((item) => (
                            <TabsTrigger key={item.value} value={item.value}>
                                {item.label}
                            </TabsTrigger>
                        ))}
                    </TabsList>
                    {tabs.map((item) => (
                        <TabsContent key={item.value} value={item.value} className="mt-5">
                            <p className="font-sans text-sm text-copy-muted sm:text-base">
                                {item.description}
                            </p>
                            {tab === item.value && <PortfolioResults type={item.value} q={q} />}
                        </TabsContent>
                    ))}
                </Tabs>
            </div>
        </main>
    );
}

function PortfolioResults({ type, q }: { type: Tab; q: string }) {
    const pathname = usePathname();
    const load = useCallback(
        (signal: AbortSignal) =>
            browsePortfolios({ visibility: type, q, signal }).then(({ data }) => data),
        [type, q]
    );
    const { state, reload } = useResource(load);

    return (
        <div className="mt-5">
            <Resource
                state={state}
                reload={reload}
                loadingLabel="載入備審"
                renderError={(error) => {
                    if (isVerificationRequired(error)) {
                        return <VerificationRequired feature="交換備審" />;
                    }
                    if (error instanceof ApiError && error.status === 401) {
                        return (
                            <Card className="flex flex-col items-start gap-4 sm:flex-row sm:items-center">
                                <p className="min-w-0 flex-1 font-sans text-base text-copy-muted">
                                    交換備審僅開放已驗證的特選生瀏覽，請先登入。
                                </p>
                                <Button asChild size="base" className="w-full gap-2 sm:w-auto">
                                    <Link href={loginHref(`${pathname}?type=exchange`)}>
                                        <LogIn aria-hidden className="size-4" />
                                        登入
                                    </Link>
                                </Button>
                            </Card>
                        );
                    }
                    return undefined;
                }}
            >
                {(portfolios) =>
                    portfolios.length === 0 ? (
                        <EmptyState
                            title={q ? `找不到符合「${q}」的備審` : "目前還沒有分享的備審"}
                            description="試試其他關鍵字，或成為第一個分享備審的人。"
                            action={
                                <Button asChild size="base">
                                    <Link href={portfolioEditorHref()}>分享我的備審</Link>
                                </Button>
                            }
                        />
                    ) : (
                        <>
                            <p className="mb-3 font-sans text-sm text-copy-muted">
                                共 {portfolios.length} 份 · 資料更新：
                                {formatNumericDate(
                                    portfolios.map(portfolioUpdatedAt).sort().at(-1)!
                                )}
                            </p>
                            <ul className="flex flex-col gap-3">
                                {portfolios.map((portfolio) => (
                                    <li key={portfolio.id}>
                                        <PortfolioCard portfolio={portfolio} />
                                    </li>
                                ))}
                            </ul>
                        </>
                    )
                }
            </Resource>
        </div>
    );
}
