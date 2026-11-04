import type { Metadata } from "next";
import ArticlesView from "./articles-view";

export const metadata: Metadata = {
    title: "文章發佈 | S.T.A 特殊選才資源網"
};

export default function Page() {
    return <ArticlesView />;
}
