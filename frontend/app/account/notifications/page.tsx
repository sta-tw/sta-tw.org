import type { Metadata } from "next";
import NotificationsView from "./notifications-view";

export const metadata: Metadata = {
    title: "通知設定 | S.T.A 特殊選才資源網"
};

export default function Page() {
    return <NotificationsView />;
}
