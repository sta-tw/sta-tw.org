import type { Metadata } from "next";
import MailRoutesView from "./mail-routes-view";

export const metadata: Metadata = {
    title: "Mail 分類管理 | S.T.A 管理後台",
    robots: { index: false, follow: false }
};

export default function AdminMailRoutesPage() {
    return <MailRoutesView />;
}
