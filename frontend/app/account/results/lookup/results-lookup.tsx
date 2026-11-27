"use client";

import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { ArrowLeft, Info } from "lucide-react";
import { useCallback } from "react";
import Button from "../../../components/button";
import Field from "../../../components/field";
import Select from "../../../components/select";
import { EmptyState, Resource, StatusBadge, type BadgeTone } from "../../../components/status";
import {
    getApplicationResult,
    listApplications,
    type Application,
    type ApplicationResult,
    type ResultStatus
} from "../../../lib/api/applications";
import { ApiError } from "../../../lib/api/types";
import { useResource } from "../../../lib/use-resource";
import AccountPage from "../../account-page";
import { isVerificationRequired, VerificationRequired } from "../../verification-required";

const REGISTER_HREF = "/account/results/";

const loadApplications = (signal: AbortSignal) =>
    listApplications({ signal }).then(({ data }) => data);

const resultStatus: Record<ResultStatus, { label: string; tone: BadgeTone }> = {
    admitted: { label: "正取", tone: "success" },
    waitlisted: { label: "備取", tone: "pending" },
    rejected: { label: "未錄取", tone: "danger" },
    unknown: { label: "結果未明", tone: "neutral" }
};

const nameOf = (application: Application) =>
    `${application.school_name} ${application.program_name}`;

export default function ResultsLookup() {
    const { state, reload } = useResource(loadApplications);

    return (
        <AccountPage
            title="交叉查榜查詢"
            description="查看已公布的個人結果與後端提供的遞補參考資料。"
            actions={
                <Button asChild variant="secondary" size="base" className="gap-2">
                    <Link href={REGISTER_HREF}>
                        <ArrowLeft aria-hidden className="size-4" />
                        回到查榜登記
                    </Link>
                </Button>
            }
        >
            <Resource
                state={state}
                reload={reload}
                renderError={(error) =>
                    isVerificationRequired(error) ? (
                        <VerificationRequired feature="交叉查榜" />
                    ) : undefined
                }
            >
                {(applications) =>
                    applications.length === 0 ? (
                        <EmptyState
                            title="還沒有登記報考校系"
                            description="先登記報考校系並提交准考證號碼；榜單公布並完成比對後，就能查看個人結果。"
                            action={
                                <Button asChild size="base">
                                    <Link href={REGISTER_HREF}>前往查榜登記</Link>
                                </Button>
                            }
                        />
                    ) : (
                        <Lookup applications={applications} />
                    )
                }
            </Resource>
        </AccountPage>
    );
}

function Lookup({ applications }: { applications: Application[] }) {
    const router = useRouter();
    const pathname = usePathname();
    const requested = useSearchParams().get("id");
    const selected = applications.find((item) => item.id === requested) ?? applications[0];

    return (
        <>
            <Field label="選擇校系" className="max-w-xl">
                <Select
                    value={selected.id}
                    options={applications.map((item) => ({
                        value: item.id,
                        label: `${item.academic_year} 學年度・${nameOf(item)}`
                    }))}
                    onChange={(event) =>
                        router.replace(`${pathname}?id=${encodeURIComponent(event.target.value)}`, {
                            scroll: false
                        })
                    }
                />
            </Field>
            <ResultDetails key={selected.id} application={selected} />
        </>
    );
}

/** A 404 means the backend has no result to return for this application yet. */
const notFoundAs =
    <T,>(fallback: T) =>
    (cause: unknown): T => {
        if (cause instanceof ApiError && cause.status === 404) return fallback;
        throw cause;
    };

function ResultDetails({ application }: { application: Application }) {
    const load = useCallback(
        async (signal: AbortSignal) =>
            getApplicationResult(application.id, { signal }).then(
                ({ data }) => data,
                notFoundAs<ApplicationResult | null>(null)
            ),
        [application.id]
    );
    const { state, reload } = useResource(load);

    return (
        <section
            aria-label={`${nameOf(application)} 的查榜結果`}
            className="flex flex-col gap-6 border-t border-ink/10 pt-6"
        >
            <Resource state={state} reload={reload} loadingLabel="載入查榜結果">
                {(result) =>
                    result ? (
                        <>
                            <ResultSummary result={result} />
                            <Reminder />
                        </>
                    ) : (
                        <EmptyState
                            title="目前沒有可查詢的榜單結果"
                            description="榜單可能尚未公布，或後端還沒有比對到這個校系的准考證號碼。"
                        />
                    )
                }
            </Resource>
        </section>
    );
}

function ResultSummary({ result }: { result: ApplicationResult }) {
    const status = resultStatus[result.result_status];
    const stats: { label: string; value: string; hint?: string }[] = [];
    if (result.official_rank !== undefined) {
        stats.push({ label: "官方名次", value: `${status.label} ${result.official_rank}` });
    }
    if (result.quota !== undefined) stats.push({ label: "招生名額", value: `${result.quota} 名` });
    if (result.result_status === "waitlisted") {
        stats.push({
            label: "前方考生",
            value: `${result.front_candidate_count} 人`,
            hint: `其中 ${result.front_response_count} 人已填寫意願`
        });
    }
    if (result.position_after_declines !== undefined) {
        stats.push({ label: "扣除放棄後順位", value: `第 ${result.position_after_declines} 位` });
    }

    return (
        <div className="flex flex-col gap-5">
            <div className="flex flex-wrap items-end gap-x-6 gap-y-3">
                {result.result_status === "waitlisted" &&
                result.reference_probability !== undefined ? (
                    <div>
                        <p className="font-sans text-base text-copy-muted">你的參考上岸機率</p>
                        <p className="font-sans text-5xl font-medium tracking-tight text-ink">
                            {result.reference_probability.toFixed(2)}%
                        </p>
                    </div>
                ) : (
                    <p className="font-sans text-2xl font-medium text-ink">
                        {result.result_status === "admitted"
                            ? "恭喜正取！記得填寫報到意願，讓備取的同學能及早知道結果。"
                            : `榜單結果：${status.label}`}
                    </p>
                )}
                <StatusBadge tone={status.tone}>{status.label}</StatusBadge>
            </div>
            {stats.length > 0 && (
                <dl className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
                    {stats.map((stat) => (
                        <div
                            key={stat.label}
                            className="rounded-[var(--radius-control)] bg-ink/5 px-4 py-3"
                        >
                            <dt className="font-sans text-sm text-copy-muted">{stat.label}</dt>
                            <dd className="mt-1 font-sans text-2xl font-medium text-ink">
                                {stat.value}
                            </dd>
                            {stat.hint && (
                                <dd className="mt-1 font-sans text-sm text-copy-muted">
                                    {stat.hint}
                                </dd>
                            )}
                        </div>
                    ))}
                </dl>
            )}
        </div>
    );
}

function Reminder() {
    return (
        <aside className="flex items-start gap-3 rounded-[var(--radius-panel)] bg-accent-green/60 px-4 py-3 font-sans text-sm leading-relaxed text-ink sm:text-base">
            <Info aria-hidden className="mt-0.5 size-5 shrink-0" />
            <p>
                S.T.A
                提醒你：結果與遞補參考資料僅供參考。請以招生學校公告為準，勿私下詢問其他考生是否願意放棄。
            </p>
        </aside>
    );
}
