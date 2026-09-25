import type { Metadata } from "next";
import AdminShell from "./admin-shell";

export const metadata: Metadata = {
    title: "管理後台 | S.T.A 特殊選才資源網",
    description: "S.T.A 管理後台。",
    robots: { index: false, follow: false }
};

export default function AdminLayout({ children }: { children: React.ReactNode }) {
    return <AdminShell>{children}</AdminShell>;
}
