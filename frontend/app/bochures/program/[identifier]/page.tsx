import type { Metadata } from "next";
import { notFound } from "next/navigation";
import BrochureProgramView from "../program-view";
import { getProgramForPage } from "../../../lib/admissions-server";
import type { AdmissionProgram } from "../../../lib/api/admissions";

type PageProps = {
    params: Promise<{ identifier: string }>;
};

function programDescription(program: AdmissionProgram) {
    const details = [
        `${program.academic_year} 學年度`,
        program.school_name,
        program.admission_program_name,
        program.admission_quota > 0 ? `招生名額 ${program.admission_quota} 名` : ""
    ].filter(Boolean);
    const target = program.special_talent_target.trim();
    if (target && target !== "-") details.push(`特殊選才對象：${target}`);
    const description = `${details.join("，")}。查看招生方式、考試項目與重要時程。`;
    return description.length > 180 ? `${description.slice(0, 177)}…` : description;
}

export async function generateMetadata({ params }: PageProps): Promise<Metadata> {
    const { identifier } = await params;
    const program = await getProgramForPage(identifier);
    if (!program) {
        return { title: "找不到簡章資料 | S.T.A 特殊選才資源網" };
    }

    return {
        title: `${program.academic_year} 學年度 ${program.school_name} ${program.admission_program_name} 簡章 | S.T.A`,
        description: programDescription(program),
        alternates: {
            canonical: `/bochures/program/${encodeURIComponent(identifier)}/`
        }
    };
}

export default async function BrochureProgramPage({ params }: PageProps) {
    const { identifier } = await params;
    const program = await getProgramForPage(identifier);
    if (!program) notFound();

    return <BrochureProgramView program={program} />;
}
