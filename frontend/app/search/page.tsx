import type { Metadata } from "next";
import StatusPage from "../components/status-page";

export const metadata: Metadata = {
    title: "簡章搜尋建置中 | S.T.A 特殊選才資源網",
    description: "簡章搜尋頁面正在建置中。"
};

export default function SearchPage() {
    return (
        <StatusPage
            eyebrow="Coming Soon"
            title="簡章搜尋正在整理中"
            description="我們正在把校系簡章、招生條件與重要時程整理成更容易篩選的搜尋體驗。完成前，可以先從文章總覽查看特殊選才準備方向。"
        />
    );
}
