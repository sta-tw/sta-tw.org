import type { Metadata } from "next";
import StatusPage from "./components/status-page";

export const metadata: Metadata = {
    title: "找不到頁面 | S.T.A 特殊選才資源網",
    description: "這個頁面不存在或已經移動。"
};

export default function NotFound() {
    return (
        <StatusPage
            eyebrow="404"
            title="這個頁面暫時找不到"
            description="網址可能輸入錯誤，或內容已經移動。你可以回到首頁重新開始，或前往文章總覽尋找特殊選才相關資訊。"
            actions={[
                { label: "回到首頁", href: "/", variant: "primary" },
                { label: "文章總覽", href: "/articles", variant: "secondary" }
            ]}
        />
    );
}
