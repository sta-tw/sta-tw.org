import type { Metadata } from "next";
import ForumView from "./forum-view";

export const metadata: Metadata = {
    title: "討論區 | S.T.A 特殊選才資源網",
    description: "依學年度與校系分開的討論空間，交流特殊選才準備經驗。"
};

export default function ForumPage() {
    return (
        <main className="mx-auto w-full max-w-screen-xl px-5 py-12 sm:px-6 sm:py-16 lg:px-16 lg:py-20">
            <ForumView />
        </main>
    );
}
