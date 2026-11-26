import type { Metadata } from "next";
import { Suspense } from "react";
import { LoadingState } from "../../components/status";
import ArticlesView from "./articles-view";

export const metadata: Metadata = {
    title: "文章發佈 | S.T.A 特殊選才資源網"
};

export default function Page() {
    return (
        // ArticlesView reads the editor's ?notice= from useSearchParams.
        <Suspense fallback={<LoadingState />}>
            <ArticlesView />
        </Suspense>
    );
}
