import type { Metadata } from "next";
import SearchView from "./search-view";

export const metadata: Metadata = {
    title: "搜尋 | S.T.A 特殊選才資源網",
    description: "搜尋學校、校系管道與心得文章。"
};

export default function SearchPage() {
    return (
        <main className="mx-auto w-full max-w-screen-xl px-5 py-12 sm:px-6 sm:py-16 lg:px-16 lg:py-20">
            <SearchView />
        </main>
    );
}
