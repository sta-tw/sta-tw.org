import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { articles, getArticle } from "../../data/articles";
import ArticleHeader from "../article-header";

type ArticlePageProps = {
    params: Promise<{ slug: string }>;
};

export function generateStaticParams() {
    return articles.map(({ slug }) => ({ slug }));
}

export async function generateMetadata({ params }: ArticlePageProps): Promise<Metadata> {
    const { slug } = await params;
    const article = getArticle(slug);

    if (!article) {
        return { title: "找不到文章 | S.T.A 特殊選才資源網" };
    }

    return {
        title: `${article.title} | S.T.A 特殊選才資源網`,
        description: article.summary
    };
}

export default async function ArticlePage({ params }: ArticlePageProps) {
    const { slug } = await params;
    const article = getArticle(slug);

    if (!article) notFound();

    return (
        <main className="article-dots flex-1 bg-surface">
            <article className="mx-auto w-full max-w-screen-lg px-5 pt-12 pb-16 sm:px-6 sm:pt-16 sm:pb-20 lg:px-16 lg:pt-20 lg:pb-24">
                <ArticleHeader
                    title={article.title}
                    readTime={article.readTime}
                    date={article.date}
                />

                <div className="mx-auto mt-10 max-w-3xl font-sans text-base leading-8 text-ink/85 sm:mt-12 sm:text-lg sm:leading-9">
                    {article.content.map((section, index) => (
                        <section
                            key={section.heading ?? index}
                            className="mb-10 last:mb-0 sm:mb-12"
                        >
                            {section.heading && (
                                <h2 className="mb-4 text-2xl leading-tight font-medium text-ink sm:mb-5 sm:text-3xl">
                                    {section.heading}
                                </h2>
                            )}
                            {section.items && (
                                <ul className="mb-6 list-disc space-y-2 pl-6 marker:text-ink/60">
                                    {section.items.map((item) => (
                                        <li key={item}>{item}</li>
                                    ))}
                                </ul>
                            )}
                            {section.paragraphs?.map((paragraph) => (
                                <p key={paragraph} className="mb-6 last:mb-0">
                                    {paragraph}
                                </p>
                            ))}
                        </section>
                    ))}
                </div>
            </article>
        </main>
    );
}
