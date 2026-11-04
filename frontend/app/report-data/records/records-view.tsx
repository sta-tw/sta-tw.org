"use client";

import Link from "next/link";
import { useEffect, useMemo, useRef, useState } from "react";
import { ArrowLeft, ExternalLink, Search } from "lucide-react";
import Button from "../../components/button";
import { Card } from "../../components/card";
import Field from "../../components/field";
import { Input } from "../../components/input";
import Select from "../../components/select";
import { EmptyState, LoadingState } from "../../components/status";
import { currentRocYear, rocYearOptions } from "../../lib/format";
import {
    getPublishedBrochurePreviewURL,
    listAllAdmissionPrograms,
    listPublishedBrochures,
    type AdmissionProgram
} from "../../lib/api/admissions";
import { programHref } from "../../lib/routes";

const latestAcademicYear = Math.max(116, currentRocYear());
const academicYears = Array.from(
    { length: latestAcademicYear - 103 },
    (_, index) => latestAcademicYear - index
);
const yearOptions = [
    { value: "all", label: "全部學年度" },
    ...rocYearOptions({ from: 104, to: latestAcademicYear, suffix: " 學年度" })
];
const pageSize = 40;

const countFields = [
    { key: "admission_quota", label: "招生名額" },
    { key: "applicant_count", label: "報名" },
    { key: "interview_count", label: "面試" },
    { key: "admitted_count", label: "正取" },
    { key: "waitlisted_count", label: "備取" },
    { key: "promoted_count", label: "遞補" }
] as const;

type BrochureAvailability =
    | { status: "loading" }
    | { status: "loaded"; years: number[] }
    | { status: "error" };

function normalizeSearch(value: string) {
    return value.replace(/\s+/g, "").replace(/台/g, "臺").toLocaleLowerCase("zh-TW");
}

function displayCount(value: string | number | undefined, academicYear: number) {
    const text = String(value ?? "").trim();
    if (["", "-", "—", "待統計", "尚未公告", "暫無資料"].includes(text)) {
        return academicYear >= latestAcademicYear ? "尚未公告" : "暫無資料";
    }
    return text;
}

function BrochureStatus({
    program,
    availability
}: {
    program: AdmissionProgram;
    availability: BrochureAvailability | undefined;
}) {
    if (!availability || availability.status === "loading") {
        return <span className="text-copy-muted">查詢中</span>;
    }
    if (availability.status === "error") {
        return (
            <span className="text-copy-muted" title="目前無法確認簡章檔案狀態">
                無法確認
            </span>
        );
    }

    if (!availability.years.includes(program.academic_year)) {
        return <span className="text-copy-muted">尚無</span>;
    }

    return (
        <a
            href={getPublishedBrochurePreviewURL(program.academic_year, program.school_code)}
            target="_blank"
            rel="noopener noreferrer"
            className="font-bold text-ink underline underline-offset-4 hover:text-ink/70"
            aria-label={`預覽 ${program.academic_year} 學年度簡章`}
            title={`預覽 ${program.academic_year} 學年度簡章`}
        >
            預覽
        </a>
    );
}

export default function ReportDataRecordsView() {
    const [programs, setPrograms] = useState<AdmissionProgram[] | null>(null);
    const [error, setError] = useState(false);
    const [reloadToken, setReloadToken] = useState(0);
    const [yearFilter, setYearFilter] = useState("all");
    const [search, setSearch] = useState("");
    const [page, setPage] = useState(1);
    const [brochureAvailability, setBrochureAvailability] = useState<
        Record<string, BrochureAvailability>
    >({});
    const brochureRequests = useRef(new Set<string>());

    useEffect(() => {
        const controller = new AbortController();
        const years = academicYears;
        void Promise.all(
            years.map(async (academicYear) => {
                const response = await listAllAdmissionPrograms({
                    academicYear,
                    signal: controller.signal
                });
                return response.data;
            })
        )
            .then((results) => {
                const allPrograms = results.flat();
                allPrograms.sort((left, right) => {
                    if (left.academic_year !== right.academic_year) {
                        return right.academic_year - left.academic_year;
                    }
                    const schoolOrder = left.school_name.localeCompare(right.school_name, "zh-TW");
                    return (
                        schoolOrder ||
                        left.admission_program_name.localeCompare(
                            right.admission_program_name,
                            "zh-TW"
                        )
                    );
                });
                setPrograms(allPrograms);
                setError(false);
            })
            .catch(() => {
                if (!controller.signal.aborted) setError(true);
            });
        return () => controller.abort();
    }, [reloadToken]);

    const filteredPrograms = useMemo(() => {
        if (!programs) return [];
        const normalizedSearch = normalizeSearch(search.trim());
        return programs.filter((program) => {
            const matchesYear =
                yearFilter === "all" || program.academic_year === Number(yearFilter);
            const searchableText = normalizeSearch(
                `${program.school_name} ${program.admission_program_name} ${program.academic_year}`
            );
            return matchesYear && (!normalizedSearch || searchableText.includes(normalizedSearch));
        });
    }, [programs, search, yearFilter]);

    const pageCount = Math.max(1, Math.ceil(filteredPrograms.length / pageSize));
    const visiblePrograms = useMemo(
        () => filteredPrograms.slice((page - 1) * pageSize, page * pageSize),
        [filteredPrograms, page]
    );
    const visibleSchoolCodes = useMemo(
        () =>
            Array.from(
                new Set(visiblePrograms.map((program) => program.school_code).filter(Boolean))
            ),
        [visiblePrograms]
    );

    useEffect(() => {
        if (visibleSchoolCodes.length === 0) return;

        const schoolsToLoad = visibleSchoolCodes.filter(
            (schoolCode) => !brochureRequests.current.has(schoolCode)
        );
        if (schoolsToLoad.length === 0) return;
        for (const schoolCode of schoolsToLoad) brochureRequests.current.add(schoolCode);

        setBrochureAvailability((current) => {
            const next = { ...current };
            for (const schoolCode of schoolsToLoad) next[schoolCode] = { status: "loading" };
            return next;
        });

        void Promise.all(
            schoolsToLoad.map(async (schoolCode) => {
                try {
                    const response = await listPublishedBrochures(schoolCode);
                    return {
                        schoolCode,
                        result: {
                            status: "loaded",
                            years: response.data.map((item) => item.academic_year)
                        } as BrochureAvailability
                    };
                } catch {
                    return { schoolCode, result: { status: "error" } as BrochureAvailability };
                }
            })
        ).then((results) => {
            setBrochureAvailability((current) => {
                const next = { ...current };
                for (const result of results) {
                    if (result) next[result.schoolCode] = result.result;
                }
                return next;
            });
        });
    }, [visibleSchoolCodes]);

    function changeYear(value: string) {
        setYearFilter(value);
        setPage(1);
    }

    function changeSearch(value: string) {
        setSearch(value);
        setPage(1);
    }

    function reload() {
        setPrograms(null);
        setError(false);
        setPage(1);
        brochureRequests.current.clear();
        setBrochureAvailability({});
        setReloadToken((token) => token + 1);
    }

    return (
        <main className="article-dots flex-1 border-y border-ink/5">
            <div className="mx-auto w-full max-w-screen-xl px-5 py-10 sm:px-6 sm:py-14 lg:px-16 lg:py-16">
                <header className="mx-auto mb-8 max-w-3xl text-center sm:mb-10">
                    <p className="font-sans text-sm font-bold tracking-[0.16em] text-ink/55">
                        S.T.A COMMUNITY DATA
                    </p>
                    <h1 className="mt-3 font-serif text-4xl leading-tight tracking-[-0.04em] text-ink sm:text-5xl lg:text-6xl">
                        已收錄資料
                    </h1>
                    <p className="mt-4 font-sans leading-7 text-copy-muted sm:text-lg">
                        查詢學年度、學校或科系，確認本站公開的招生數字與各年度簡章。簡章欄位可直接預覽已上架的檔案。
                    </p>
                    <Button asChild variant="secondary" size="sm" className="mt-5 gap-2">
                        <Link href="/report-data">
                            <ArrowLeft aria-hidden className="size-4" />
                            回到資料回報
                        </Link>
                    </Button>
                </header>

                <section className="mx-auto max-w-6xl" aria-label="已收錄招生資料清單">
                    <Card className="grid gap-3 p-4 sm:grid-cols-[minmax(10rem,0.35fr)_minmax(16rem,1fr)] sm:p-5">
                        <Field label="學年度">
                            <Select
                                value={yearFilter}
                                onChange={(event) => changeYear(event.target.value)}
                                options={yearOptions}
                            />
                        </Field>
                        <Field label="搜尋學校或科系">
                            <div className="relative">
                                <Search
                                    aria-hidden
                                    className="pointer-events-none absolute top-1/2 left-4 size-4 -translate-y-1/2 text-copy-muted"
                                />
                                <Input
                                    type="search"
                                    value={search}
                                    onChange={(event) => changeSearch(event.target.value)}
                                    placeholder="輸入學校或科系名稱"
                                    className="pl-10"
                                />
                            </div>
                        </Field>
                    </Card>

                    <div
                        className="mt-4 flex flex-wrap items-center justify-between gap-2 font-sans text-sm text-copy-muted"
                        aria-live="polite"
                    >
                        {programs ? (
                            <p>
                                共 {filteredPrograms.length.toLocaleString("zh-TW")} 筆資料
                                {yearFilter === "all"
                                    ? "，涵蓋目前可查詢的學年度"
                                    : `，${yearFilter} 學年度`}
                            </p>
                        ) : error ? (
                            <p className="text-danger">目前無法載入資料。</p>
                        ) : (
                            <p>正在載入已公開的招生資料…</p>
                        )}
                        <p>
                            空白欄位：{latestAcademicYear} 學年度顯示「尚未公告」；
                            {latestAcademicYear - 1} 學年度及更早年度顯示「暫無資料」。
                        </p>
                    </div>

                    {error ? (
                        <div className="mt-4 rounded-[var(--radius-small)] border border-ink/10 bg-surface/80 p-6 text-center">
                            <p className="font-sans text-sm leading-6 text-copy-muted">
                                資料暫時無法載入，請稍後再試。
                            </p>
                            <Button
                                type="button"
                                variant="secondary"
                                onClick={reload}
                                size="sm"
                                className="mt-3"
                            >
                                重新載入
                            </Button>
                        </div>
                    ) : programs === null ? (
                        <LoadingState />
                    ) : filteredPrograms.length === 0 ? (
                        <div className="mt-4">
                            <EmptyState title="找不到符合條件的資料" />
                        </div>
                    ) : (
                        <>
                            <div className="mt-4 hidden overflow-x-auto rounded-[var(--radius-small)] border border-ink/10 bg-surface/80 md:block">
                                <table className="w-full min-w-[72rem] border-collapse text-left font-sans text-sm">
                                    <caption className="sr-only">
                                        已收錄校系招生數字與各年度簡章狀態
                                    </caption>
                                    <thead className="bg-ink/[0.03] text-ink">
                                        <tr className="border-b border-ink/10">
                                            <th
                                                scope="col"
                                                className="px-3 py-3 font-bold whitespace-nowrap"
                                            >
                                                學年度
                                            </th>
                                            <th scope="col" className="px-3 py-3 font-bold">
                                                學校與科系
                                            </th>
                                            <th
                                                scope="col"
                                                className="px-3 py-3 text-center font-bold whitespace-nowrap"
                                            >
                                                簡章
                                            </th>
                                            {countFields.map((field) => (
                                                <th
                                                    key={field.key}
                                                    scope="col"
                                                    className="px-3 py-3 text-center font-bold whitespace-nowrap"
                                                >
                                                    {field.label}
                                                </th>
                                            ))}
                                        </tr>
                                    </thead>
                                    <tbody className="divide-y divide-ink/10 text-ink/80">
                                        {visiblePrograms.map((program) => (
                                            <tr
                                                key={program.program_identifier}
                                                className="transition-colors hover:bg-accent-green-strong/20"
                                            >
                                                <td className="px-3 py-3 whitespace-nowrap">
                                                    {program.academic_year}
                                                </td>
                                                <td className="px-3 py-3">
                                                    <Link
                                                        href={programHref(
                                                            program.program_identifier
                                                        )}
                                                        className={`group inline-flex w-fit max-w-full min-w-0 items-start font-bold text-ink underline-offset-4 focus-ring hover:underline`}
                                                    >
                                                        <span className="min-w-0">
                                                            <span className="inline-flex items-center gap-1">
                                                                {program.school_name}
                                                                <ExternalLink
                                                                    aria-hidden
                                                                    className="size-3.5 shrink-0 opacity-50 group-hover:opacity-100"
                                                                />
                                                            </span>
                                                            <span className="mt-0.5 block font-normal text-copy-muted">
                                                                {program.admission_program_name}
                                                            </span>
                                                        </span>
                                                    </Link>
                                                </td>
                                                <td className="px-3 py-3 text-center whitespace-nowrap">
                                                    <BrochureStatus
                                                        program={program}
                                                        availability={
                                                            brochureAvailability[
                                                                program.school_code
                                                            ]
                                                        }
                                                    />
                                                </td>
                                                {countFields.map((field) => (
                                                    <td
                                                        key={field.key}
                                                        className="px-3 py-3 text-center whitespace-nowrap tabular-nums"
                                                    >
                                                        {displayCount(
                                                            program[field.key],
                                                            program.academic_year
                                                        )}
                                                    </td>
                                                ))}
                                            </tr>
                                        ))}
                                    </tbody>
                                </table>
                            </div>

                            <ul className="mt-4 grid gap-3 md:hidden">
                                {visiblePrograms.map((program) => (
                                    <li
                                        key={program.program_identifier}
                                        className="rounded-[var(--radius-small)] border border-ink/10 bg-surface/80 p-4"
                                    >
                                        <div className="flex items-start justify-between gap-3">
                                            <div className="min-w-0">
                                                <Link
                                                    href={programHref(program.program_identifier)}
                                                    className={`inline-flex w-fit max-w-full min-w-0 flex-col items-start font-sans leading-6 font-bold text-ink underline-offset-4 focus-ring hover:underline`}
                                                >
                                                    <span className="font-sans text-xs font-bold text-ink/55">
                                                        {program.academic_year} 學年度
                                                    </span>
                                                    <span className="inline-flex items-center gap-1">
                                                        {program.school_name}
                                                        <ExternalLink
                                                            aria-hidden
                                                            className="size-3.5 shrink-0 text-ink/50"
                                                        />
                                                    </span>
                                                    <span className="font-bold">
                                                        {program.admission_program_name}
                                                    </span>
                                                </Link>
                                            </div>
                                        </div>
                                        <dl className="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 border-t border-ink/10 pt-3 font-sans text-sm">
                                            <div className="col-span-2 flex items-center justify-between gap-2">
                                                <dt className="text-copy-muted">簡章</dt>
                                                <dd className="font-bold text-ink">
                                                    <BrochureStatus
                                                        program={program}
                                                        availability={
                                                            brochureAvailability[
                                                                program.school_code
                                                            ]
                                                        }
                                                    />
                                                </dd>
                                            </div>
                                            {countFields.map((field) => (
                                                <div
                                                    key={field.key}
                                                    className="flex justify-between gap-2"
                                                >
                                                    <dt className="text-copy-muted">
                                                        {field.label}
                                                    </dt>
                                                    <dd className="font-bold text-ink tabular-nums">
                                                        {displayCount(
                                                            program[field.key],
                                                            program.academic_year
                                                        )}
                                                    </dd>
                                                </div>
                                            ))}
                                        </dl>
                                    </li>
                                ))}
                            </ul>

                            {pageCount > 1 ? (
                                <nav
                                    className="mt-5 flex items-center justify-center gap-3 font-sans text-sm"
                                    aria-label="資料清單分頁"
                                >
                                    <Button
                                        type="button"
                                        variant="secondary"
                                        onClick={() =>
                                            setPage((current) => Math.max(1, current - 1))
                                        }
                                        disabled={page === 1}
                                        size="sm"
                                    >
                                        上一頁
                                    </Button>
                                    <span
                                        aria-live="polite"
                                        className="text-copy-muted tabular-nums"
                                    >
                                        {page} / {pageCount}
                                    </span>
                                    <Button
                                        type="button"
                                        variant="secondary"
                                        onClick={() =>
                                            setPage((current) => Math.min(pageCount, current + 1))
                                        }
                                        disabled={page === pageCount}
                                        size="sm"
                                    >
                                        下一頁
                                    </Button>
                                </nav>
                            ) : null}
                        </>
                    )}
                </section>
            </div>
        </main>
    );
}
