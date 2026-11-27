"use client";

import Link from "next/link";
import { useSearchParams, useRouter } from "next/navigation";
import { useCallback, useState } from "react";
import Button from "../../../components/button";
import Field from "../../../components/field";
import FileUpload from "../../../components/file-upload";
import { Input } from "../../../components/input";
import Select from "../../../components/select";
import { ErrorText, Resource } from "../../../components/status";
import {
    listApplications,
    applicationLabel,
    type Application
} from "../../../lib/api/applications";
import {
    createPortfolioProject,
    getMyPortfolio,
    listPortfolioProjects,
    PORTFOLIO_FILE_ACCEPT,
    PORTFOLIO_FILE_MAX_BYTES,
    PORTFOLIO_TITLE_MAX_LENGTH,
    submitPortfolioFile,
    uploadPortfolioFile,
    type Portfolio
} from "../../../lib/api/portfolio";
import { formatBytes } from "../../../lib/format";
import { useAction, useResource } from "../../../lib/use-resource";
import AccountPage from "../../account-page";
import SubmitReviewDialog from "../submit-review-dialog";

const overviewHref = "/account/portfolio/";
const maxSizeLabel = formatBytes(PORTFOLIO_FILE_MAX_BYTES);

type EditorData = {
    applications: Application[];
    usedApplicationIDs: Set<string>;
    portfolio: Portfolio | null;
};

export default function PortfolioEditor() {
    const id = useSearchParams().get("id")?.trim() ?? "";
    const load = useCallback(
        async (signal: AbortSignal): Promise<EditorData> => {
            const [{ data: applications }, { data: projects }, portfolio] = await Promise.all([
                listApplications({ signal }),
                listPortfolioProjects({ signal }),
                id ? getMyPortfolio(id, { signal }).then(({ data }) => data) : Promise.resolve(null)
            ]);
            return {
                applications,
                usedApplicationIDs: new Set(projects.map((project) => project.application_id)),
                portfolio
            };
        },
        [id]
    );
    const { state, reload } = useResource(load);

    return (
        <AccountPage
            title={id ? "上傳備審新版本" : "新增備審資料"}
            description="正式後端以已確認申請校系建立專案；每次上傳都會成為一個檔案版本，送審通過後公開。"
        >
            <Resource state={state} reload={reload}>
                {(data) => <EditorForm key={data.portfolio?.id ?? "new"} {...data} />}
            </Resource>
        </AccountPage>
    );
}

function EditorForm({ applications, usedApplicationIDs, portfolio }: EditorData) {
    const router = useRouter();
    const [applicationID, setApplicationID] = useState(portfolio?.application_id ?? "");
    const [title, setTitle] = useState(portfolio?.title === "-" ? "" : (portfolio?.title ?? ""));
    const [file, setFile] = useState<File | null>(null);
    const [createdProjectID, setCreatedProjectID] = useState<string | null>(null);
    const [showErrors, setShowErrors] = useState(false);
    const { pending, error, setError, run } = useAction();

    const availableApplications = applications.filter(
        (application) =>
            application.id === portfolio?.application_id || !usedApplicationIDs.has(application.id)
    );
    const currentFile = portfolio?.items[0];
    const errors = {
        application: applicationID ? null : "請選擇已確認的申請校系。",
        file: file ? null : "請選擇要上傳的檔案。"
    };
    const valid = !errors.application && !errors.file;

    function selectFile(selected: File) {
        if (selected.size > PORTFOLIO_FILE_MAX_BYTES) {
            setError(`檔案超過 ${maxSizeLabel} 上限。`);
            setFile(null);
            return;
        }
        setError(null);
        setFile(selected);
    }

    async function save(submit: boolean) {
        setShowErrors(true);
        if (!valid || !file) return;
        await run(async () => {
            let projectID = portfolio?.id ?? createdProjectID;
            if (!projectID) {
                const { data: project } = await createPortfolioProject({
                    application_id: applicationID,
                    title: title.trim() || "備審資料"
                });
                projectID = project.id;
                setCreatedProjectID(projectID);
            }
            const { data: uploaded } = await uploadPortfolioFile(projectID, file);
            if (submit) await submitPortfolioFile(uploaded.id);
            router.push(overviewHref);
        });
    }

    return (
        <form
            noValidate
            className="flex flex-col gap-6"
            onSubmit={(event) => {
                event.preventDefault();
                void save(false);
            }}
        >
            <Field label="已確認的申請校系" required error={showErrors ? errors.application : null}>
                <Select
                    value={applicationID}
                    placeholder="請選擇"
                    disabled={Boolean(portfolio || createdProjectID)}
                    options={availableApplications.map((application) => ({
                        value: application.id,
                        label: applicationLabel(application)
                    }))}
                    onChange={(event) => setApplicationID(event.target.value)}
                />
                {availableApplications.length === 0 && !portfolio && (
                    <p className="mt-2 font-sans text-sm text-copy-muted">
                        目前沒有可建立備審專案的已確認申請校系。
                    </p>
                )}
            </Field>

            <Field label="專案名稱" hint="選填；建立後會沿用，後端目前不提供改名。">
                <Input
                    value={title}
                    maxLength={PORTFOLIO_TITLE_MAX_LENGTH}
                    disabled={Boolean(portfolio || createdProjectID)}
                    placeholder="備審資料"
                    onChange={(event) => setTitle(event.target.value)}
                />
            </Field>

            {currentFile && (
                <p className="font-sans text-sm text-copy-muted">
                    目前最新版本：第 {currentFile.version_number} 版（{currentFile.file_name}
                    ）。上傳後會建立新版本。
                </p>
            )}
            <Field
                label={portfolio ? "上傳新版本" : "備審檔案"}
                required
                hint={`每次上傳都會建立新版本。單檔上限 ${maxSizeLabel}，可上傳 PDF、Office 文件、圖片及文字檔。`}
                error={showErrors ? errors.file : null}
            >
                <FileUpload
                    accept={PORTFOLIO_FILE_ACCEPT}
                    onFileSelected={selectFile}
                    fileName={file?.name}
                    disabled={pending !== null}
                />
            </Field>

            {createdProjectID && (
                <p className="font-sans text-sm text-copy-muted">
                    備審專案已建立；若上傳中斷，重新選擇檔案即可繼續。
                </p>
            )}
            <ErrorText>{error}</ErrorText>
            {showErrors && !valid && <ErrorText>請完成必填欄位。</ErrorText>}
            <div className="flex flex-col-reverse gap-3 border-t border-ink/10 pt-6 sm:flex-row sm:justify-end">
                <Button asChild variant="secondary" size="base">
                    <Link href={overviewHref}>取消</Link>
                </Button>
                <Button type="submit" variant="secondary" size="base" disabled={pending !== null}>
                    {pending ? "上傳中…" : "儲存為未公開版本"}
                </Button>
                <SubmitReviewDialog
                    trigger={
                        <Button type="button" size="base" disabled={pending !== null || !file}>
                            {pending ? "處理中…" : "上傳並送審"}
                        </Button>
                    }
                    onConfirm={() => save(true)}
                />
            </div>
        </form>
    );
}
