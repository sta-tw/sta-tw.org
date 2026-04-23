import type { ArticlePreview } from "../lib/article-list";
import HashtagBadge from "./hashtag-badge";

type ArticleHashtagsSectionProps = {
  articles: ArticlePreview[];
};

export default function ArticleHashtagsSection({
  articles,
}: ArticleHashtagsSectionProps) {
  const hashtags = Array.from(new Set(articles.flatMap((article) => article.tags)));

  return (
    <section
      className="bg-[linear-gradient(180deg,#f7f3e8_0%,#f8f5ed_58%,#ffffff_100%)] pb-16 sm:pb-20 lg:pb-24"
      aria-labelledby="articles-hashtags-heading"
    >
      <div className="mx-auto max-w-screen-xl px-5 sm:px-6 lg:px-16">
        <div className="border-t border-ink/10 pt-8 sm:pt-10 lg:pt-12">
          <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between sm:gap-6">
            <div className="max-w-2xl">
              <h2
                id="articles-hashtags-heading"
                className="font-serif text-3xl leading-tight text-ink sm:text-4xl"
              >
                全部 Hashtags
              </h2>
              <p className="mt-2 font-sans text-sm leading-6 font-medium text-copy-muted sm:text-base">
                快速瀏覽這個頁面涵蓋的主題標籤。
              </p>
            </div>

            <span className="shrink-0 self-start rounded-full bg-accent-yellow px-3 py-1 text-sm font-semibold text-ink">
              {hashtags.length} 個標籤
            </span>
          </div>

          <ul className="mt-6 flex flex-wrap gap-3">
            {hashtags.map((tag) => (
              <li key={tag}>
                <HashtagBadge tag={tag} />
              </li>
            ))}
          </ul>
        </div>
      </div>
    </section>
  );
}
