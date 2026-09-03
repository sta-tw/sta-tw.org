import type { Metadata } from "next";
import StatusPage from "../components/status-page";

export const metadata: Metadata = {
    title: "登入註冊建置中 | S.T.A 特殊選才資源網",
    description: "登入與註冊頁面正在建置中。"
};

export default function LoginPage() {
    return (
        <StatusPage
            eyebrow="Coming Soon"
            title="會員功能正在建置中"
            description="登入、註冊與個人化收藏功能還在串接與測試。現階段仍可直接瀏覽站上的文章內容，後續會補上更完整的會員體驗。"
        />
    );
}
