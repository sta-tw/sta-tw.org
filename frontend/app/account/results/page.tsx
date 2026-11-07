import type { Metadata } from "next";
import ResultsView from "./results-view";

export const metadata: Metadata = {
    title: "交叉查榜 | S.T.A 特殊選才資源網"
};

export default function Page() {
    return <ResultsView />;
}
