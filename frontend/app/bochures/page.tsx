import type { Metadata } from "next";
import BrochureSearch from "../components/brochure-search";
import { brochureFilters } from "../data/brochure-filters";
import { searchBrochures, type BrochureSearchFilters } from "../data/brochures";

export const metadata: Metadata = {
    title: "簡章搜尋 | S.T.A 特殊選才資源網",
    description: "依照校系與招生條件搜尋特殊選才簡章。"
};

type BrochuresPageProps = {
    searchParams: Promise<{ [key: string]: string | string[] | undefined }>;
};

function firstValue(value: string | string[] | undefined) {
    return typeof value === "string" ? value : undefined;
}

export default async function BrochuresPage({ searchParams }: BrochuresPageProps) {
    const params = await searchParams;
    const query = firstValue(params.q);
    const filters: BrochureSearchFilters = query ? { q: query } : {};

    brochureFilters.forEach((filter) => {
        const value = firstValue(params[filter.id]);
        if (value === "required" || value === "not-required") {
            filters[filter.id] = value;
        }
    });

    return <BrochureSearch filters={filters} results={searchBrochures(filters)} />;
}
