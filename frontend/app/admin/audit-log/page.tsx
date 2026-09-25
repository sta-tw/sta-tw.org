import type { Metadata } from "next";
import AuditLogView from "./audit-log-view";

export const metadata: Metadata = {
    title: "稽核紀錄 | S.T.A 管理後台",
    robots: { index: false, follow: false }
};

export default function AdminAuditLogPage() {
    return <AuditLogView />;
}
