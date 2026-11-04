import type { Metadata } from "next";
import PortfolioView from "./portfolio-view";

export const metadata: Metadata = {
    title: "備審上傳 | S.T.A 特殊選才資源網"
};

export default function Page() {
    return <PortfolioView />;
}
