"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";
import BrochureDetailTabs from "../../components/brochure-detail-tabs";
import {
    getAdmissionProgram,
    getAdmissionProgramHistory,
    getPublishedBrochureDownload,
    type AdmissionProgram
} from "../../lib/api/admissions";
import { ApiError } from "../../lib/api/types";
import {
    admissionProgramPreviews,
    admissionProgramToBrochure,
    programHistoryFromYears
} from "../../lib/admissions-brochures";
import type { BrochureHistory } from "../../lib/brochure-types";

type ProgramViewState = {
    identifier: string;
    program: AdmissionProgram | null;
    downloadUrl?: string;
    history: BrochureHistory[];
    error: string | null;
};

export default function BrochureProgramView() {
    const searchParams = useSearchParams();
    const identifier = searchParams.get("identifier")?.trim() ?? "";
    const [state, setState] = useState<ProgramViewState>({
        identifier: "",
        program: null,
        history: [],
        error: null
    });

    useEffect(() => {
        const controller = new AbortController();

        if (!identifier) {
            return () => controller.abort();
        }

        getAdmissionProgram(identifier, { signal: controller.signal })
            .then(async (response) => {
                if (controller.signal.aborted) return;
                let nextDownloadUrl: string | undefined;

                // A program can be published before its private PDF is
                // uploaded. Keep the program page usable when the download
                // endpoint correctly returns 404 in that case.
                try {
                    const download = await getPublishedBrochureDownload(
                        response.data.academic_year,
                        response.data.school_code,
                        { signal: controller.signal }
                    );
                    nextDownloadUrl = download.url;
                } catch {
                    nextDownloadUrl = undefined;
                }

                let nextHistory: BrochureHistory[] = [];
                try {
                    const historyResponse = await getAdmissionProgramHistory(
                        response.data.school_code,
                        response.data.program_code,
                        { signal: controller.signal }
                    );
                    nextHistory = programHistoryFromYears(historyResponse.data);
                } catch {
                    nextHistory = [];
                }

                if (!controller.signal.aborted) {
                    setState({
                        identifier,
                        program: response.data,
                        downloadUrl: nextDownloadUrl,
                        history: nextHistory,
                        error: null
                    });
                }
            })
            .catch((cause: unknown) => {
                if (controller.signal.aborted) return;
                setState({
                    identifier,
                    program: null,
                    history: [],
                    error:
                        cause instanceof ApiError && cause.status === 404
                            ? "找不到這筆已公開的招生資料。"
                            : cause instanceof ApiError
                              ? cause.message
                              : "目前無法取得後端招生資料，請稍後再試。"
                });
            });

        return () => controller.abort();
    }, [identifier]);

    const isLoading = Boolean(identifier) && state.identifier !== identifier;
    const program = state.identifier === identifier ? state.program : null;
    const error = state.identifier === identifier ? state.error : null;
    const downloadUrl = state.identifier === identifier ? state.downloadUrl : undefined;
    const history = state.identifier === identifier ? state.history : [];

    if (isLoading) {
        return (
            <main className="article-dots flex flex-1 items-center justify-center bg-surface px-5 py-20">
                <p className="font-sans text-base text-ink/65">正在從後端載入招生資料…</p>
            </main>
        );
    }

    if (error || !program) {
        return (
            <main className="article-dots flex flex-1 items-center justify-center bg-surface px-5 py-20">
                <div className="max-w-md text-center">
                    <p className="font-sans text-lg font-medium text-ink">無法載入招生資料</p>
                    <p className="mt-2 font-sans text-base leading-7 text-ink/65">
                        {error ?? "目前沒有可顯示的資料。"}
                    </p>
                    <Link
                        href="/bochures"
                        className="mt-6 inline-flex rounded-full bg-button px-4 py-2 font-sans text-sm font-medium text-button-foreground hover:bg-button-hover"
                    >
                        返回簡章搜尋
                    </Link>
                </div>
            </main>
        );
    }

    const brochure = { ...admissionProgramToBrochure(program), history };
    const externalLinkPreviews = admissionProgramPreviews(program);

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
                    schoolCode={program.school_code}
                />

                <aside className="mt-10 w-fit max-w-full rounded-[var(--radius-small)] bg-accent-green/45 px-4 py-2 font-sans text-sm leading-relaxed text-ink/75 sm:mt-12 sm:text-base">
                    有想要查找的資訊沒有公布在 S.T.A 嗎？或者手邊有資料想提供？歡迎來信到
                    <a
                        href="mailto:sta.bochures@googlegroups.com"
                        className="ml-1 underline decoration-ink/35 underline-offset-2 hover:text-ink"
                    >
                        sta.bochures@googlegroups.com
                    </a>
                </aside>
            </article>
        </main>
    );
}
