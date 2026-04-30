"use client";

import { useDeferredValue, useState } from "react";
import type { ArticlePreview } from "../lib/article-list";
import ArticleCard from "./article-card";
import ArticleSearchBar from "./article-search-bar";

type ArticlesSectionProps = {
    articles: ArticlePreview[];
};

export default function ArticlesSection({ articles }: ArticlesSectionProps) {
    const [query, setQuery] = useState("");
    const deferredQuery = useDeferredValue(query);
    const normalizedQuery = deferredQuery.trim().toLocaleLowerCase();

    const filteredArticles = normalizedQuery
        ? articles.filter((article) =>
              [
                  article.category,
                  article.title,
                  article.excerpt,
                  article.tags.join(" "),
                  article.publishedAt,
                  article.readTime
              ]
                  .join(" ")
                  .toLocaleLowerCase()
                  .includes(normalizedQuery)
          )
        : articles;

    return (
        <section
            className="bg-[linear-gradient(180deg,#ffffff_0%,#f7f3e8_100%)] pt-8 pb-16 sm:pt-10 sm:pb-20 lg:pt-12 lg:pb-24"
            aria-labelledby="articles-list-heading"
        >
            <div className="mx-auto flex max-w-screen-xl flex-col gap-8 px-5 sm:px-6 lg:gap-10 lg:px-16">
                <h2 id="articles-list-heading" className="sr-only">
                    文章列表
                </h2>

                <ArticleSearchBar
                    query={query}
                    resultsCount={filteredArticles.length}
                    totalCount={articles.length}
                    onQueryChange={setQuery}
                />

                {filteredArticles.length > 0 ? (
                    <div className="grid grid-cols-1 gap-6 lg:grid-cols-2 xl:gap-8">
                        {filteredArticles.map((article) => (
                            <ArticleCard key={article.id} article={article} />
                        ))}
                    </div>
                ) : (
                    <div className="rounded-[var(--radius-panel)] border border-dashed border-ink/15 bg-white/75 px-6 py-12 text-center shadow-[var(--shadow-card)] sm:px-10">
                        <h3 className="font-serif text-3xl leading-tight text-ink">
                            找不到符合的文章
                        </h3>
                        <p className="mx-auto mt-3 max-w-xl font-sans text-base leading-7 font-medium text-copy-muted">
                            試試看更短的關鍵字，例如「備審」、「面試」或「作品集」，通常會比完整句子更容易找到對應內容。
                        </p>
                    </div>
                )}
            </div>
        </section>
    );
}
