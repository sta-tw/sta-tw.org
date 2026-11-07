"use client";

import { ChevronDown, Lock } from "lucide-react";
import { Collapsible } from "radix-ui";
import { useState } from "react";
import { Card } from "../../components/card";
import { listApplications, type Application } from "../../lib/api/applications";
import AccountPage from "../account-page";
import { EmptyState, Resource, StatusBadge, type BadgeTone } from "../../components/status";
import { useResource } from "../../lib/use-resource";
import { isVerificationRequired, VerificationRequired } from "../verification-required";
import AddApplicationsForm from "./add-applications-form";
import ResultPanel from "./result-panel";

const loadApplications = (signal: AbortSignal) =>
    listApplications({ signal }).then(({ data }) => data);

const applicationStatus: Record<string, { label: string; tone: BadgeTone }> = {
    draft: { label: "草稿", tone: "neutral" },
    confirmed: { label: "已確認", tone: "success" },
    withdrawn: { label: "已撤回", tone: "neutral" },
    archived: { label: "已封存", tone: "neutral" }
};

export default function ResultsView() {
    const { state, reload, setData } = useResource(loadApplications);

    return (
        <AccountPage
            title="交叉查榜"
            description="登記你申請的校系與准考證號碼，榜單公布後可查看名次、遞補情形與參考錄取機率。"
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
                {(applications) => (
                    <>
                        <AddApplicationsForm
                            existing={applications}
                            onCreated={(created) => setData((items) => [...items, ...created])}
                        />
                        <section
                            aria-labelledby="applications-title"
                            className="flex flex-col gap-4"
                        >
                            <h2
                                id="applications-title"
                                className="font-sans text-2xl font-medium text-ink"
                            >
                                我的申請校系
                            </h2>
                            {applications.length === 0 ? (
                                <EmptyState
                                    title="還沒有登記申請校系"
                                    description="用上方的搜尋加入你申請的校系，就能在榜單公布後交叉查榜。"
                                />
                            ) : (
                                <ul className="flex flex-col gap-4">
                                    {applications.map((application) => (
                                        <li key={application.id}>
                                            <ApplicationItem application={application} />
                                        </li>
                                    ))}
                                </ul>
                            )}
                        </section>
                    </>
                )}
            </Resource>
        </AccountPage>
    );
}

function ApplicationItem({ application }: { application: Application }) {
    const [open, setOpen] = useState(false);
    // Keep the panel (and its fetched result) mounted after the first open
    // so collapsing and re-opening doesn't refetch.
    const [opened, setOpened] = useState(false);
    const status = applicationStatus[application.status] ?? {
        label: application.status,
        tone: "neutral" as const
    };

    return (
        <Collapsible.Root
            open={open}
            onOpenChange={(next) => {
                setOpen(next);
                if (next) setOpened(true);
            }}
        >
            <Card className="p-0 sm:p-0">
                <Collapsible.Trigger className="group flex w-full cursor-pointer items-center gap-4 rounded-[var(--radius-panel)] p-5 text-left focus-ring sm:p-6">
                    <div className="min-w-0 flex-1">
                        <p className="font-sans text-sm text-copy-muted">
                            {application.academic_year} 學年度
                        </p>
                        <h3 className="font-sans text-xl leading-snug font-medium break-words text-ink">
                            {application.school_name} {application.program_name}
                        </h3>
                        <div className="mt-2 flex flex-wrap items-center gap-2">
                            <StatusBadge tone={status.tone}>{status.label}</StatusBadge>
                            {application.locked_at && (
                                <span className="inline-flex items-center gap-1 font-sans text-sm text-copy-muted">
                                    <Lock aria-hidden className="size-3.5" />
                                    已鎖定
                                </span>
                            )}
                        </div>
                    </div>
                    <span className="hidden font-sans text-sm text-copy-muted sm:inline">
                        {open ? "收合" : "查看結果"}
                    </span>
                    <ChevronDown
                        aria-hidden
                        className="size-5 shrink-0 text-copy-muted transition-transform group-data-[state=open]:rotate-180"
                    />
                </Collapsible.Trigger>
                <Collapsible.Content
                    forceMount={opened || undefined}
                    className="border-t border-ink/10 p-5 sm:p-6"
                >
                    <ResultPanel applicationID={application.id} />
                </Collapsible.Content>
            </Card>
        </Collapsible.Root>
    );
}
