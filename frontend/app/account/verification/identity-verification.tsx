"use client";

import { useState } from "react";
import Button from "../../components/button";
import { Card } from "../../components/card";
import {
    getVerificationRequests,
    revokeVerification,
    withdrawVerificationRequest,
    type VerificationRequestWithDetails
} from "../../lib/api/verification";
import ConfirmDialog from "../../components/confirm-dialog";
import { labelOf } from "../../components/select";
import { ErrorText, LoadingState, Resource } from "../../components/status";
import { useAction, useResource } from "../../lib/use-resource";
import { formatDateTime } from "../../lib/format";
import {
    findSchoolByCode,
    useSchoolDirectory,
    type SchoolDirectory
} from "../../components/school-picker";
import VerificationForm from "./verification-form";
import { documentTypeOptions, identityTypeOptions } from "./verification-options";

const INACTIVE = new Set(["cancelled", "expired", "revoked"]);

function Summary({
    request,
    schools
}: {
    request: VerificationRequestWithDetails;
    schools: SchoolDirectory;
}) {
    const school =
        findSchoolByCode(schools.schools, request.school_code)?.school_name ?? request.school_code;
    const rows: [string, string | undefined][] = [
        ["申請的身分別", labelOf(identityTypeOptions, request.identity_type)],
        ["提交文件類別", labelOf(documentTypeOptions, request.document_type)],
        ["學校", school],
        ["學年度", `${request.academic_year} 學年度`],
        ["學群", request.academic_fields?.join("、")],
        ["已上傳文件", `${request.document_count} 份`],
        ["提交時間", formatDateTime(request.created_at)]
    ];
    return (
        <dl className="grid gap-x-6 gap-y-3 rounded-[var(--radius-small)] bg-ink/[0.03] p-4 font-sans text-base sm:grid-cols-[auto_minmax(0,1fr)] sm:p-5">
            {rows
                .filter(([, value]) => value)
                .map(([label, value]) => (
                    <div key={label} className="contents">
                        <dt className="text-sm text-copy-muted sm:text-base">{label}</dt>
                        <dd className="mb-2 min-w-0 break-words text-ink sm:mb-0">{value}</dd>
                    </div>
                ))}
        </dl>
    );
}

const statusMessages = {
    pending: "您已完成提交，審核工作約需一至三個工作天。",
    approved: "您已驗證成功！",
    rejected: "很抱歉，您的身分驗證未通過審核。請確認文件清晰、資訊完整後重新提交。"
} as const;

const loadRequests = (signal: AbortSignal) =>
    getVerificationRequests({ signal }).then(({ data }) => data);

export default function IdentityVerification() {
    const { state, reload } = useResource(loadRequests);
    // Loaded alongside the requests; the summary and form both need it.
    const schools = useSchoolDirectory();
    return (
        <Card
            title="身分驗證"
            description="上傳學生證或錄取通知，通過審核後即可使用備審上傳與交叉查榜等功能。"
            className="flex flex-col gap-5"
        >
            <Resource state={state} reload={reload} loadingLabel="正在載入驗證狀態">
                {(requests) => (
                    <VerificationStatus requests={requests} schools={schools} onChanged={reload} />
                )}
            </Resource>
        </Card>
    );
}

function VerificationStatus({
    requests,
    schools,
    onChanged
}: {
    requests: VerificationRequestWithDetails[];
    schools: SchoolDirectory;
    onChanged: () => void;
}) {
    const [editing, setEditing] = useState(false);
    const { error: actionError, run } = useAction();

    const latest = requests[0];
    const current = latest && !INACTIVE.has(latest.status) ? latest : undefined;

    async function runAction(action: (id: string) => Promise<void>) {
        if (current && (await run(() => action(current.id)))) {
            setEditing(false);
            onChanged();
        }
    }

    if (!current || editing) {
        // The form resolves school names to codes, so it waits for the list.
        if (schools.status === "loading") return <LoadingState label="正在載入學校清單" />;
        return (
            <VerificationForm
                schools={schools}
                initial={editing ? current : undefined}
                replacing={editing && current?.status === "pending" ? current.id : undefined}
                onCancel={editing ? () => setEditing(false) : undefined}
                onSubmitted={() => {
                    setEditing(false);
                    onChanged();
                }}
            />
        );
    }

    const status = current.status as keyof typeof statusMessages;
    return (
        <div className="flex flex-col gap-5">
            <p
                role="status"
                className={`font-sans text-lg font-medium ${status === "rejected" ? "text-danger" : "text-ink"}`}
            >
                {statusMessages[status]}
            </p>
            <Summary request={current} schools={schools} />
            <div className="flex flex-wrap gap-3">
                {status === "pending" && (
                    <>
                        <Button
                            type="button"
                            variant="secondary"
                            size="base"
                            onClick={() => setEditing(true)}
                        >
                            變更
                        </Button>
                        <ConfirmDialog
                            trigger={
                                <Button type="button" variant="secondary" size="base">
                                    取消上傳
                                </Button>
                            }
                            title="取消這次上傳？"
                            description="這次提交的文件將不會被審核，你可以之後再重新提交。"
                            confirmLabel="取消上傳"
                            onConfirm={() => void runAction(withdrawVerificationRequest)}
                        />
                    </>
                )}
                {status === "approved" && (
                    <>
                        <Button
                            type="button"
                            variant="secondary"
                            size="base"
                            onClick={() => setEditing(true)}
                        >
                            更新身分驗證
                        </Button>
                        <ConfirmDialog
                            trigger={
                                <Button type="button" variant="danger" size="base">
                                    刪除已驗證身分
                                </Button>
                            }
                            title="刪除已驗證身分？"
                            description="刪除後將無法使用需要驗證身分的功能，如需恢復必須重新提交並等待審核。"
                            confirmLabel="刪除"
                            onConfirm={() => void runAction(revokeVerification)}
                        />
                    </>
                )}
                {status === "rejected" && (
                    <Button type="button" onClick={() => setEditing(true)}>
                        重新提交
                    </Button>
                )}
            </div>
            <ErrorText>{actionError}</ErrorText>
        </div>
    );
}
