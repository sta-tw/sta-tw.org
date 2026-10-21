import type { Metadata } from "next";
import AdmissionsContributionsView from "./contributions-view";

export const metadata: Metadata = {
    title: "資料回報 | S.T.A 管理後台",
    robots: { index: false, follow: false }
};

export default function AdmissionsContributionsPage() {
    return <AdmissionsContributionsView />;
}
