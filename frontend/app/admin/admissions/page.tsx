import type { Metadata } from "next";
import AdmissionsView from "./admissions-view";

export const metadata: Metadata = {
    title: "簡章管理 | S.T.A 管理後台",
    robots: { index: false, follow: false }
};

export default function AdminAdmissionsPage() {
    return <AdmissionsView />;
}
