import type { Metadata } from "next";
import { Suspense } from "react";
import ExperienceView from "./experience-view";

export const metadata: Metadata = {
    title: "心得文章 | S.T.A 特殊選才資源網",
    description: "閱讀特殊選才申請經驗與準備心得。"
};

function LoadingState() {
    return (
        <main className="article-dots flex flex-1 items-center justify-center bg-surface px-5 py-20">
            <p className="font-sans text-base text-ink/65">正在載入心得文章…</p>
        </main>
    );
}

export default function ExperienceArticlePage() {
    return (
        <Suspense fallback={<LoadingState />}>
            <ExperienceView />
        </Suspense>
    );
}
