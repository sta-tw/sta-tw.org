import type { Metadata } from "next";
import { Suspense } from "react";
import BrochureProgramView from "./program-view";

export const metadata: Metadata = {
    title: "招生資料 | S.T.A 特殊選才資源網",
    description: "查看由 S.T.A 後端招生資料系統提供的校系與簡章資訊。"
};

function LoadingState() {
    return (
        <main className="article-dots flex flex-1 items-center justify-center bg-surface px-5 py-20">
            <p className="font-sans text-base text-ink/65">正在載入招生資料…</p>
        </main>
    );
}

export default function BrochureProgramPage() {
    return (
        <Suspense fallback={<LoadingState />}>
            <BrochureProgramView />
        </Suspense>
    );
}
