import type { Metadata } from "next";
import ArticlesHeroSection from "../components/articles-hero-section";

export const metadata: Metadata = {
  title: "文章總覽 | S.T.A 特殊選才資源網",
  description: "瀏覽特殊選才相關文章、經驗與整理內容。",
};

export default function ArticlesPage() {
  return (
    <main className="bg-surface">
      <ArticlesHeroSection />
    </main>
  );
}
