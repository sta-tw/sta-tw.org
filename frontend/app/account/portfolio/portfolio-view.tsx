"use client";

import Link from "next/link";
import { ChevronDown, Download, FileText } from "lucide-react";
import { Collapsible } from "radix-ui";
import { useCallback, useMemo, useState } from "react";
import Button from "../../components/button";
import { Card } from "../../components/card";
import ConfirmDialog from "../../components/confirm-dialog";
import Field from "../../components/field";
import FileUpload from "../../components/file-upload";
import { Input } from "../../components/input";
import Select from "../../components/select";
import { applicationLabel, listApplications, type Application } from "../../lib/api/applications";
import {
    createPortfolioProject,
    getPortfolioFileDownload,
    hidePortfolioFile,
    listPortfolioFiles,
    listPortfolioProjects,
    PORTFOLIO_FILE_ACCEPT,
    PORTFOLIO_FILE_MAX_BYTES,
    PORTFOLIO_TITLE_MAX_LENGTH,
    submitPortfolioFile,
    unpublishPortfolioFile,
    uploadPortfolioFile,
    type PortfolioFile,
    type PortfolioFileStatus,
    type PortfolioProject
} from "../../lib/api/portfolio";
import AccountPage from "../account-page";
import {
    EmptyState,
    ErrorText,
    Resource,
    StatusBadge,
    type BadgeTone
} from "../../components/status";
import { useAction, useResource } from "../../lib/use-resource";
import { formatBytes, formatDateTime } from "../../lib/format";
import { isVerificationRequired, VerificationRequired } from "../verification-required";

const loadOverview = (signal: AbortSignal) =>
    Promise.all([listPortfolioProjects({ signal }), listApplications({ signal })]).then(
        ([projects, applications]) => ({
            projects: projects.data,
            applications: applications.data
        })
    );

const fileStatus: Record<PortfolioFileStatus, { label: string; tone: BadgeTone }> = {
    hidden: { label: "未公開", tone: "neutral" },
    pending_review: { label: "審核中", tone: "pending" },
    published: { label: "已公開", tone: "success" },
    unpublished: { label: "已下架", tone: "neutral" },
    rejected: { label: "未通過審核", tone: "danger" }
};

const maxSizeLabel = formatBytes(PORTFOLIO_FILE_MAX_BYTES);

export default function PortfolioView() {
    const { state, reload, setData } = useResource(loadOverview);

    return (
        <AccountPage
            title="備審上傳"
            description="為每個申請校系上傳備審資料，送審通過後會公開給其他特選生參考。"
        >
            <Resource
                state={state}
                reload={reload}
                renderError={(error) =>
                    isVerificationRequired(error) ? (
                        <VerificationRequired feature="備審上傳" />
                    ) : undefined
                }
            >
                {(overview) => (
                    <PortfolioOverview
                        {...overview}
                        onCreated={(project) =>
                            setData((data) => ({ ...data, projects: [project, ...data.projects] }))
                        }
                    />
                )}
            </Resource>
        </AccountPage>
    );
}

function PortfolioOverview({
    projects,
    applications,
    onCreated
}: {
    projects: PortfolioProject[];
    applications: Application[];
    onCreated: (project: PortfolioProject) => void;
}) {
    const applicationsById = useMemo(
        () => new Map(applications.map((application) => [application.id, application])),
        [applications]
    );
    const available = useMemo(() => {
        const taken = new Set(projects.map((project) => project.application_id));
        return applications.filter(
            (application) => application.status === "confirmed" && !taken.has(application.id)
        );
    }, [applications, projects]);

    if (applications.length === 0) {
        return (
            <EmptyState
                title="還沒有申請校系"
                description="備審資料需要對應到一個申請校系，請先在交叉查榜新增你申請的校系。"
                action={
                    <Button asChild size="base">
                        <Link href="/account/results">新增申請校系</Link>
                    </Button>
                }
            />
        );
    }

    return (
        <>
            <CreateProjectForm applications={available} onCreated={onCreated} />
            {projects.length === 0 ? (
                <EmptyState
                    title="還沒有備審專案"
                    description="選擇一個申請校系建立專案後，就能開始上傳檔案。"
                />
            ) : (
                <ul className="flex flex-col gap-6">
                    {projects.map((project) => (
                        <li key={project.id}>
                            <ProjectCard
                                project={project}
                                application={applicationsById.get(project.application_id)}
                            />
                        </li>
                    ))}
                </ul>
            )}
        </>
    );
}

function CreateProjectForm({
    applications,
    onCreated
}: {
    applications: Application[];
    onCreated: (project: PortfolioProject) => void;
}) {
    const [applicationID, setApplicationID] = useState("");
    const [title, setTitle] = useState("");
    const { pending, error, run } = useAction();
    const saving = pending !== null;

    if (applications.length === 0) return null;

    function submit(event: React.FormEvent) {
        event.preventDefault();
        run(async () => {
            const { data } = await createPortfolioProject({
                application_id: applicationID,
                title: title.trim()
            });
            onCreated(data);
            setApplicationID("");
            setTitle("");
        });
    }

    return (
        <Card title="建立備審專案">
            <form onSubmit={submit} className="mt-5 grid gap-5 md:grid-cols-2">
                <Field label="申請校系" required>
                    <Select
                        value={applicationID}
                        onChange={(event) => setApplicationID(event.target.value)}
                        placeholder="請選擇"
                        options={applications.map((application) => ({
                            value: application.id,
                            label: applicationLabel(application)
                        }))}
                    />
                </Field>
                <Field label="專案名稱" hint="選填，例如「書面備審資料」。">
                    <Input
                        value={title}
                        maxLength={PORTFOLIO_TITLE_MAX_LENGTH}
                        onChange={(event) => setTitle(event.target.value)}
                    />
                </Field>
                <div className="flex flex-wrap items-center gap-4 md:col-span-2">
                    <Button type="submit" disabled={saving || !applicationID} size="base">
                        {saving ? "建立中…" : "建立專案"}
                    </Button>
                    <ErrorText>{error}</ErrorText>
                </div>
            </form>
        </Card>
    );
}

/** Collapsed by default; the files load on first expand and stay mounted
 * after that (same as ResultPanel in 交叉查榜). */
function ProjectCard({
    project,
    application
}: {
    project: PortfolioProject;
    application?: Application;
}) {
    const [open, setOpen] = useState(false);
    const [opened, setOpened] = useState(false);
    const title = project.title === "-" ? "備審資料" : project.title;

    return (
        <Collapsible.Root
            open={open}
            onOpenChange={(next) => {
                setOpen(next);
                if (next) setOpened(true);
            }}
        >
            <Card aria-label={title} className="p-0 sm:p-0">
                <Collapsible.Trigger className="group flex w-full cursor-pointer items-center gap-4 rounded-[var(--radius-panel)] p-5 text-left focus-ring sm:p-6">
                    <div className="min-w-0 flex-1">
                        <h2 className="font-sans text-xl font-medium break-words text-ink sm:text-2xl">
                            {title}
                        </h2>
                        {application && (
                            <p className="mt-1 font-sans text-base text-copy-muted">
                                {applicationLabel(application)}
                            </p>
                        )}
                    </div>
                    <span className="hidden font-sans text-sm text-copy-muted sm:inline">
                        {open ? "收合" : "管理檔案"}
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
                    <ProjectFiles projectID={project.id} />
                </Collapsible.Content>
            </Card>
        </Collapsible.Root>
    );
}

function ProjectFiles({ projectID }: { projectID: string }) {
    const loadFiles = useCallback(
        (signal: AbortSignal) =>
            listPortfolioFiles(projectID, { signal }).then(({ data }) =>
                [...data].sort((a, b) => b.version_number - a.version_number)
            ),
        [projectID]
    );
    const { state, reload, setData } = useResource(loadFiles);
    const { pending, error, setError, run } = useAction();
    const uploading = pending === "upload";

    function upload(file: File) {
        if (file.size > PORTFOLIO_FILE_MAX_BYTES) {
            setError(`檔案超過 ${maxSizeLabel} 上限。`);
            return;
        }
        run("upload", async () => {
            const { data } = await uploadPortfolioFile(projectID, file);
            setData((files) => [data, ...files]);
        });
    }

    const change = (
        file: PortfolioFile,
        action: (id: string) => Promise<{ data: PortfolioFile }>
    ) =>
        run(file.id, async () => {
            const { data } = await action(file.id);
            setData((files) => files.map((item) => (item.id === data.id ? data : item)));
        });

    function download(file: PortfolioFile) {
        // Open the tab synchronously so popup blockers allow it, then point
        // it at the presigned URL once the request resolves.
        const tab = window.open("", "_blank");
        run(`download:${file.id}`, async () => {
            try {
                const { url } = await getPortfolioFileDownload(file.id);
                if (tab) {
                    tab.opener = null;
                    tab.location.href = url;
                } else {
                    window.location.assign(url);
                }
            } catch (cause) {
                tab?.close();
                throw cause;
            }
        });
    }

    return (
        <div className="flex flex-col gap-5">
            <Field
                label="上傳新版本"
                hint={`每次上傳都會建立新版本。單檔上限 ${maxSizeLabel}，支援 PDF、Office 文件與圖片。`}
            >
                <FileUpload
                    accept={PORTFOLIO_FILE_ACCEPT}
                    onFileSelected={upload}
                    disabled={uploading}
                    label={uploading ? "上傳中…" : "選擇檔案"}
                />
            </Field>
            <ErrorText>{error}</ErrorText>
            <Resource state={state} reload={reload} loadingLabel="載入檔案">
                {(files) =>
                    files.length === 0 ? (
                        <p className="font-sans text-base text-copy-muted">尚未上傳任何檔案。</p>
                    ) : (
                        <ol className="flex flex-col divide-y divide-ink/10 border-t border-ink/10">
                            {files.map((file) => (
                                <FileRow
                                    key={file.id}
                                    file={file}
                                    busy={pending === file.id}
                                    onDownload={() => download(file)}
                                    onSubmit={() => change(file, submitPortfolioFile)}
                                    onUnpublish={() => change(file, unpublishPortfolioFile)}
                                    onHide={() => change(file, hidePortfolioFile)}
                                />
                            ))}
                        </ol>
                    )
                }
            </Resource>
        </div>
    );
}

function FileRow({
    file,
    busy,
    onDownload,
    onSubmit,
    onUnpublish,
    onHide
}: {
    file: PortfolioFile;
    busy: boolean;
    onDownload: () => void;
    onSubmit: () => void;
    onUnpublish: () => void;
    onHide: () => void;
}) {
    const status = fileStatus[file.status];
    const canSubmit = ["hidden", "unpublished", "rejected"].includes(file.status);
    const canUnpublish = file.status === "published";
    const canHide = ["published", "unpublished", "rejected"].includes(file.status);

    return (
        <li className="flex flex-col gap-3 py-4 lg:flex-row lg:items-center lg:gap-4">
            <FileText aria-hidden className="hidden size-6 shrink-0 text-copy-muted lg:block" />
            <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                    <span className="font-sans text-sm font-semibold text-ink">
                        v{file.version_number}
                    </span>
                    <StatusBadge tone={status.tone}>{status.label}</StatusBadge>
                </div>
                <p className="mt-1 font-sans text-base break-all text-ink">
                    {file.original_file_name}
                </p>
                <p className="font-sans text-sm text-copy-muted">
                    {formatBytes(file.file_size_bytes)} ·{" "}
                    <time dateTime={file.created_at}>{formatDateTime(file.created_at)}</time>
                </p>
                {file.status === "rejected" && file.rejection_reason && (
                    <p className="mt-1 font-sans text-sm text-danger">
                        未通過原因：{file.rejection_reason}
                    </p>
                )}
            </div>
            <div className="flex flex-wrap gap-2">
                <Button variant="secondary" size="sm" className="gap-2" onClick={onDownload}>
                    <Download aria-hidden className="size-4" />
                    下載
                </Button>
                {canSubmit && (
                    <ConfirmDialog
                        trigger={
                            <Button size="sm" disabled={busy}>
                                送出審核
                            </Button>
                        }
                        title="送出審核？"
                        description="審核通過後，這個版本會公開給其他使用者瀏覽與下載。之後仍可隨時下架或隱藏。"
                        confirmLabel="送出審核"
                        tone="primary"
                        onConfirm={onSubmit}
                    />
                )}
                {canUnpublish && (
                    <ConfirmDialog
                        trigger={
                            <Button variant="secondary" size="sm" disabled={busy}>
                                下架
                            </Button>
                        }
                        title="下架這個版本？"
                        description="下架後其他使用者將無法再看到這個檔案；要重新公開需再次送審。"
                        confirmLabel="下架"
                        onConfirm={onUnpublish}
                    />
                )}
                {canHide && (
                    <ConfirmDialog
                        trigger={
                            <Button variant="secondary" size="sm" disabled={busy}>
                                隱藏
                            </Button>
                        }
                        title="隱藏這個版本？"
                        description="隱藏後檔案只有你自己看得到；要重新公開需再次送審。"
                        confirmLabel="隱藏"
                        onConfirm={onHide}
                    />
                )}
            </div>
        </li>
    );
}
