import type { Metadata } from "next";

import PolicyDocument from "../components/policy-document";
import { serviceTermsPolicy } from "../lib/site-policies";

export const metadata: Metadata = {
    title: "服務條款 | S.T.A 特殊選才資源網",
    description:
        "S.T.A 特殊選才資源網用戶服務條款，說明帳號、平台服務、用戶內容、違規處置與使用者權利義務。"
};

export default function TermsOfServicePage() {
    return <PolicyDocument policy="terms" fallback={serviceTermsPolicy} />;
}
