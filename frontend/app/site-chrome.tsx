"use client";

import { usePathname } from "next/navigation";
import Footer from "./components/footer";
import Navbar from "./components/navbar";

export default function SiteChrome({ children }: { children: React.ReactNode }) {
    const pathname = usePathname();
    const isAdminRoute = pathname === "/admin" || pathname.startsWith("/admin/");

    return (
        <>
            {!isAdminRoute ? <Navbar /> : null}
            {children}
            {!isAdminRoute ? <Footer /> : null}
        </>
    );
}
