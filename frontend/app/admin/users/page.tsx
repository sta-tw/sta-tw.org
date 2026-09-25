import type { Metadata } from "next";
import UsersView from "./users-view";

export const metadata: Metadata = {
    title: "使用者管理 | S.T.A 管理後台",
    robots: { index: false, follow: false }
};

export default function AdminUsersPage() {
    return <UsersView />;
}
