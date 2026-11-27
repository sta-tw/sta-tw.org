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
            description="登記報考校系並提交准考證號碼。放榜後可查看後端提供的個人結果與遞補參考資料。"
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
                                    報考校系清單
                                </h2>
                                <p className="mt-1 font-sans text-base text-copy-muted">
                                    志願序與准考證號碼狀態目前不會由後端回傳；這裡只顯示已登記校系並提供號碼提交。
                                </p>
                            </div>
                            {applications.length === 0 ? (
                                <EmptyState
                                    title="還沒有登記報考校系"
                                    description="用上方的搜尋加入你報考的校系。"
                                />
                            ) : (
                                <PreferenceList applications={applications} />
                            )}
                        </section>
                    </>
                )}
            </Resource>
        </AccountPage>
    );
}
