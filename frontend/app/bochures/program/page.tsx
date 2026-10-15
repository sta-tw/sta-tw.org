import { notFound, permanentRedirect } from "next/navigation";

type SearchParams = Promise<{ identifier?: string | string[] }>;

type PageProps = {
    searchParams: SearchParams;
};

export default async function LegacyBrochureProgramPage({ searchParams }: PageProps) {
    const params = await searchParams;
    const value = Array.isArray(params.identifier) ? params.identifier[0] : params.identifier;
    const identifier = value?.trim() ?? "";
    if (!identifier) notFound();

    permanentRedirect(`/bochures/program/${encodeURIComponent(identifier)}/`);
}
