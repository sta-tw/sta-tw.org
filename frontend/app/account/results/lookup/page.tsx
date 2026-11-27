import type { Metadata } from "next";
import { Suspense } from "react";
import { LoadingState } from "../../../components/status";
import ResultsLookup from "./results-lookup";

export const metadata: Metadata = {
    title: "交叉查榜查詢 | S.T.A 特殊選才資源網"
};

export default function Page() {
    return (
        // The selected application is kept in ?id=.
        <Suspense fallback={<LoadingState />}>
            <ResultsLookup />
        </Suspense>
    );
}
