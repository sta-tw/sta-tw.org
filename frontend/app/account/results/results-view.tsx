"use client";

import Link from "next/link";
import { ArrowRight } from "lucide-react";
import Button from "../../components/button";
import { EmptyState, Resource } from "../../components/status";
import { listApplications } from "../../lib/api/applications";
import { useResource } from "../../lib/use-resource";
import AccountPage from "../account-page";
import { isVerificationRequired, VerificationRequired } from "../verification-required";
import AddApplicationsForm from "./add-applications-form";
import PreferenceList from "./preference-list";

const loadApplications = (signal: AbortSignal) =>
    listApplications({ signal }).then(({ data }) => data);

export default function ResultsView() {
    const { state, reload, setData } = useResource(loadApplications);

    return (
        <AccountPage
            title="交叉查榜登記"
            description="登記你報考的校系與准考證號碼，放榜後就能交叉比對名次、填寫報到意願。"
            actions={
                <Button asChild variant="secondary" size="base" className="gap-2">
                    <Link href="/account/results/lookup/">
                        查看查榜結果
                        <ArrowRight aria-hidden className="size-4" />
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
                            <div>
                                <h2
                                    id="applications-title"
                                    className="font-sans text-2xl font-medium text-ink"
                                >
                                    報考校系清單及志願序
                                </h2>
                                <p className="mt-1 font-sans text-base text-copy-muted">
                                    填寫准考證號碼後才能參與交叉查榜，並排列你的志願序。
                                </p>
                            </div>
                            {applications.length === 0 ? (
                                <EmptyState
                                    title="還沒有登記報考校系"
                                    description="用上方的搜尋加入你報考的校系。"
                                />
                            ) : (
                                <PreferenceList
                                    applications={applications}
                                    onChange={(next) => setData(() => next)}
                                />
                            )}
                        </section>
                    </>
                )}
            </Resource>
        </AccountPage>
    );
}
