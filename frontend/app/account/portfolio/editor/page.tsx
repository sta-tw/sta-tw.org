import type { Metadata } from "next";
import { Suspense } from "react";
import { LoadingState } from "../../../components/status";
import PortfolioEditor from "./portfolio-editor";

export const metadata: Metadata = {
    title: "編輯備審 | S.T.A 特殊選才資源網"
};

export default function Page() {
    return (
        // The editor reads ?id= from useSearchParams.
        <Suspense fallback={<LoadingState />}>
            <PortfolioEditor />
        </Suspense>
    );
}
