import type { Metadata } from "next";
import ReportDataRecordsView from "./records-view";

export const metadata: Metadata = {
    title: "已收錄資料 | S.T.A 特殊選才資源網",
    description: "查詢本站目前已公開的特殊選才校系與招生人數資料。"
};

export default function ReportDataRecordsPage() {
    return <ReportDataRecordsView />;
}
