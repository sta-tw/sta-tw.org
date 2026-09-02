import type { Metadata } from "next";
import BrochureSearch from "../components/brochure-search";

export const metadata: Metadata = {
    title: "簡章搜尋 | S.T.A 特殊選才資源網",
    description: "依照校系與招生條件搜尋特殊選才簡章。"
};

export default function BrochuresPage() {
    return <BrochureSearch />;
}
