import type { Metadata } from "next";
import StatusPage from "../components/status-page";

export const metadata: Metadata = {
    title: "論壇建置中 | S.T.A 特殊選才資源網",
    description: "論壇頁面正在建置中。"
};

export default function ForumPage() {
    return (
        <StatusPage
            eyebrow="Coming Soon"
            title="論壇空間準備中"
            description="我們正在規劃適合提問、交流與整理回覆的討論區。正式上線前，歡迎先加入 Discord，和其他準備特殊選才的同學交流。"
            actions={[
                { label: "回到首頁", href: "/", variant: "primary" },
                { label: "瀏覽文章", href: "/articles", variant: "secondary" }
            ]}
        />
    );
}
