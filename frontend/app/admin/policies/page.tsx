import type { Metadata } from "next";
import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import Button from "../../components/button";
import SitePolicyEditor from "../site-policy-editor";

export const metadata: Metadata = {
    title: "條款維護 | S.T.A 管理後台",
    robots: { index: false, follow: false }
};

export default function AdminPoliciesPage() {
    return (
        <div className="flex flex-col gap-8">
            <header className="flex flex-wrap items-start justify-between gap-4 border-b border-ink/10 pb-8">
                <div>
                    <p className="font-sans text-sm text-ink/60">管理模組／內容規範</p>
                    <h1 className="mt-2 font-serif text-4xl tracking-[-0.04em] text-ink sm:text-5xl">
                        條款維護
                    </h1>
                    <p className="mt-3 max-w-2xl font-sans text-sm leading-6 text-ink/60 sm:text-base">
                        集中管理服務條款與隱私權政策，更新後會同步套用到前台公開頁面。
                    </p>
                </div>
                <Button asChild variant="secondary" className="h-10 px-4 font-sans text-sm">
                    <Link href="/admin">
                        <ArrowLeft aria-hidden className="mr-2 h-4 w-4" />
                        回到管理總覽
                    </Link>
                </Button>
            </header>

            <SitePolicyEditor />
        </div>
    );
}
