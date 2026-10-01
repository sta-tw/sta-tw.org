import type { Metadata } from "next";

import PolicyDocument from "../components/policy-document";
import { privacyPolicy } from "../lib/site-policies";

export const metadata: Metadata = {
    title: "隱私權政策 | S.T.A 特殊選才資源網",
    description:
        "S.T.A 特殊選才資源網隱私權政策，說明資料蒐集、利用、保存、安全措施與使用者個人資料權利。"
};

export default function PrivacyPolicyPage() {
    return <PolicyDocument policy="privacy" fallback={privacyPolicy} />;
}
