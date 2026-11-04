import type { Metadata } from "next";
import AccountShell from "./account-shell";

export const metadata: Metadata = {
    title: "我的帳號 | S.T.A 特殊選才資源網",
    robots: { index: false, follow: false }
};

export default function AccountLayout({ children }: { children: React.ReactNode }) {
    return <AccountShell>{children}</AccountShell>;
}
