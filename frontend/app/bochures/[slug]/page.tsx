import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import BrochureDetailTabs from "../../components/brochure-detail-tabs";
import { brochures, getBrochure } from "../../data/brochures";
import { getOpenGraphPreviews } from "../../lib/open-graph";

type BrochureDetailPageProps = {
    params: Promise<{ slug: string }>;
};

export function generateStaticParams() {
    return brochures.map(({ slug }) => ({ slug }));
}

export async function generateMetadata({ params }: BrochureDetailPageProps): Promise<Metadata> {
    const { slug } = await params;
    const brochure = getBrochure(slug);

    if (!brochure) {
        return { title: "找不到簡章 | S.T.A 特殊選才資源網" };
    }

    return {
        title: `${brochure.title} | S.T.A 特殊選才資源網`,
        description: brochure.summary
    };
}

export default async function BrochureDetailPage({ params }: BrochureDetailPageProps) {
    const { slug } = await params;
    const brochure = getBrochure(slug);

    if (!brochure) notFound();

    const externalLinkPreviews = await getOpenGraphPreviews(brochure.externalLinks);

    return (
        <main className="article-dots flex-1 bg-surface">
            <article className="mx-auto w-full max-w-screen-xl px-5 py-7 sm:px-6 sm:py-10 lg:px-16 lg:py-12">
                <nav aria-label="Breadcrumb" className="font-sans text-sm text-ink/65 sm:text-base">
                    <ol className="flex flex-wrap items-center gap-x-2 gap-y-1">
                        <li>
                            <Link
                                href="/bochures"
                                className="transition-colors hover:text-ink hover:underline"
                            >
                                簡章搜尋
                            </Link>
                        </li>
                        <li aria-hidden>›</li>
                        <li>{brochure.university}</li>
                        <li aria-hidden>›</li>
                        <li aria-current="page">
                            {brochure.department}（{brochure.group}）
                        </li>
                    </ol>
                </nav>

                <header className="mt-5 sm:mt-7">
                    <h1 className="max-w-5xl font-sans text-3xl leading-tight font-medium tracking-[-0.035em] text-ink sm:text-4xl lg:text-5xl">
                        {brochure.title}
                    </h1>
                </header>

                <BrochureDetailTabs
                    brochure={brochure}
                    externalLinkPreviews={externalLinkPreviews}
                />

                <aside className="mt-10 w-fit max-w-full rounded-[var(--radius-small)] bg-accent-green/45 px-4 py-2 font-sans text-sm leading-relaxed text-ink/75 sm:mt-12 sm:text-base">
                    有想要查找的資訊沒有公布在 S.T.A 嗎？或者手邊有資料想提供？歡迎來信到
                    <a
                        href="mailto:sta.bochures@googlegroups.com"
                        className="ml-1 underline decoration-ink/35 underline-offset-2 hover:text-ink"
                    >
                        sta.bochures@googlegroups.com
                    </a>
                </aside>
            </article>
        </main>
    );
}
