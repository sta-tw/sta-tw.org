import type { Metadata } from "next";
import { Suspense } from "react";
import BrochureSearch from "../components/brochure-search";

export const metadata: Metadata = {
    title: "簡章搜尋 | S.T.A 特殊選才資源網",
    description: "依照校系與招生條件搜尋特殊選才簡章。"
};

function LoadingState() {
    return (
        <main className="article-dots flex flex-1 items-center justify-center bg-surface px-5 py-20">
            <p className="font-sans text-base text-ink/65">正在載入簡章搜尋…</p>
        </main>
    );
}

export default function BrochuresPage() {
    return (
        <Suspense fallback={<LoadingState />}>
            <BrochureSearch />
        </Suspense>
    );
}
