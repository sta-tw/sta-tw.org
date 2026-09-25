import type { Metadata } from "next";
import ArticleOverview from "../components/article-overview";

export const metadata: Metadata = {
    title: "文章總覽 | S.T.A 特殊選才資源網",
    description: "閱讀特殊選才準備方向、備審資料與面試經驗。"
};

export default function ArticleMainPage() {
    return <ArticleOverview />;
}
