import type { Metadata } from "next";
import { Suspense } from "react";
import { LoadingState } from "../components/status";
import PortfolioBrowser from "./portfolio-browser";

export const metadata: Metadata = {
    title: "交換與公開備審 | S.T.A 特殊選才資源網",
    description: "瀏覽學長姐分享的特殊選才備審資料，依校系搜尋並收藏想參考的備審。"
};

export default function PortfolioPage() {
    return (
        <Suspense
            fallback={
                <main className="article-dots flex-1">
                    <LoadingState />
                </main>
            }
        >
            <PortfolioBrowser />
        </Suspense>
    );
}
