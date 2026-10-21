import type { Metadata } from "next";
import ReportDataForm from "./report-data-form";

export const metadata: Metadata = {
    title: "招生資料回報 | S.T.A 特殊選才資源網",
    description: "直接向 S.T.A 回報特殊選才報名人數、二階與遞補資料，或提供歷屆招生簡章。"
};

export default function ReportDataPage() {
    return <ReportDataForm />;
}
