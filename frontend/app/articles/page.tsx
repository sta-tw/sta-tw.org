import type { Metadata } from "next";
import ArticleHashtagsSection from "../components/article-hashtags-section";
import ArticlesHeroSection from "../components/articles-hero-section";
import ArticlesSection from "../components/articles-section";
import { articleList } from "../lib/article-list";

export const metadata: Metadata = {
    title: "文章總覽 | S.T.A 特殊選才資源網",
    description: "瀏覽特殊選才相關文章、經驗與整理內容。"
};

export default function ArticlesPage() {
    return (
        <main className="bg-surface">
            <ArticlesHeroSection />
            <ArticlesSection articles={articleList} />
            <ArticleHashtagsSection articles={articleList} />
        </main>
    );
}
