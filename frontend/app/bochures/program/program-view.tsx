"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import BrochureDetailTabs from "../../components/brochure-detail-tabs";
import {
    getAdmissionProgramHistory,
    getPublishedBrochureDownloadURL,
    getPublishedBrochurePreviewURL,
    publishedBrochureExists,
    type AdmissionProgram
} from "../../lib/api/admissions";
import {
    admissionProgramPreviews,
    admissionProgramToBrochure,
    programHistoryFromYears
} from "../../lib/admissions-brochures";
import type { BrochureHistory } from "../../lib/brochure-types";
import { contactMailtoHref } from "../../lib/mailto";

export default function BrochureProgramView({ program }: { program: AdmissionProgram }) {
    const [history, setHistory] = useState<BrochureHistory[]>([]);
    const [brochureExists, setBrochureExists] = useState(false);

    useEffect(() => {
        const controller = new AbortController();

        // The core program is already in the server-rendered HTML. Load these
        // optional details together so neither one delays the other.
        void Promise.all([
            publishedBrochureExists(program.academic_year, program.school_code, {
                signal: controller.signal
            }),
            getAdmissionProgramHistory(program.school_code, program.program_code, {
                signal: controller.signal
            })
                .then((response) => programHistoryFromYears(response.data))
                .catch(() => [])
        ]).then(([exists, nextHistory]) => {
            if (controller.signal.aborted) return;
            setBrochureExists(exists);
            setHistory(nextHistory);
        });

        return () => controller.abort();
    }, [program.academic_year, program.program_code, program.school_code]);

    const brochure = { ...admissionProgramToBrochure(program), history };
    const externalLinkPreviews = admissionProgramPreviews(program);
    const downloadUrl = brochureExists
        ? getPublishedBrochureDownloadURL(program.academic_year, program.school_code)
        : undefined;
    const previewUrl = brochureExists
        ? getPublishedBrochurePreviewURL(program.academic_year, program.school_code)
        : undefined;

    return (
        <main className="article-dots flex-1 bg-surface">
            <article className="mx-auto w-full max-w-screen-xl px-5 py-7 sm:px-6 sm:py-10 lg:px-16 lg:py-12">
                <nav aria-label="Breadcrumb" className="font-sans text-sm text-ink/65 sm:text-base">
                    <ol className="flex flex-wrap items-center gap-x-2 gap-y-1">
                        <li>
                            <Link
                                href="/bochures"
                                className="transition-colors hover:text-ink hover:underline"
                            >
                                簡章搜尋
                            </Link>
                        </li>
                        <li aria-hidden>›</li>
                        <li>{program.school_name}</li>
                        <li aria-hidden>›</li>
                        <li aria-current="page">{program.admission_program_name}</li>
                    </ol>
                </nav>

                <header className="mt-5 sm:mt-7">
                    <p className="font-sans text-sm text-ink/60 sm:text-base">
                        {program.academic_year} 學年度・校系編號 {program.school_code}-
                        {program.program_code}
                    </p>
                    <h1 className="mt-2 max-w-5xl font-sans text-3xl leading-tight font-medium tracking-[-0.035em] text-ink sm:text-4xl lg:text-5xl">
                        {brochure.title}
                    </h1>
                </header>

                <BrochureDetailTabs
                    brochure={brochure}
                    externalLinkPreviews={externalLinkPreviews}
                    downloadUrl={downloadUrl}
                    previewUrl={previewUrl}
                    schoolCode={program.school_code}
                    currentAcademicYear={program.academic_year}
                />

                <aside className="mt-10 w-fit max-w-full rounded-[var(--radius-small)] bg-accent-green/45 px-4 py-2 font-sans text-sm leading-relaxed text-ink/75 sm:mt-12 sm:text-base">
                    有想要查找的資訊沒有公布在 S.T.A 嗎？或者手邊有資料想提供？歡迎來信到
                    <a
                        href={contactMailtoHref("brochure", {
                            academicYear: program.academic_year,
                            schoolName: program.school_name,
                            programName: program.admission_program_name
                        })}
                        className="ml-1 underline decoration-ink/35 underline-offset-2 hover:text-ink"
                    >
                        brochure@mail.sta-tw.org
                    </a>
                </aside>
            </article>
        </main>
    );
}
