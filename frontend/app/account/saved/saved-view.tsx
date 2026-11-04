"use client";

import Link from "next/link";
import { Trash2 } from "lucide-react";
import Button from "../../components/button";
import { ContentCard } from "../../components/card";
import ConfirmDialog from "../../components/confirm-dialog";
import IconButton from "../../components/icon-button";
import { listSavedPrograms, unsaveProgram, type SavedProgram } from "../../lib/api/saved-programs";
import AccountPage from "../account-page";
import { EmptyState, ErrorText, Resource } from "../../components/status";
import { useAction, useResource } from "../../lib/use-resource";
import { formatDate } from "../../lib/format";
import { programHref } from "../../lib/routes";

const loadSaved = (signal: AbortSignal) => listSavedPrograms({ signal }).then(({ data }) => data);

export default function SavedView() {
    const { state, reload, setData } = useResource(loadSaved);
    const { pending: removing, error: actionError, run } = useAction();

    const remove = (program: SavedProgram) =>
        run(program.program_identifier, async () => {
            await unsaveProgram(program.program_identifier);
            setData((items) =>
                items.filter((item) => item.program_identifier !== program.program_identifier)
            );
        });

    return (
        <AccountPage title="已儲存的項目" description="你標記為「有興趣」的校系簡章。">
            <ErrorText>{actionError}</ErrorText>
            <Resource state={state} reload={reload}>
                {(programs) =>
                    programs.length === 0 ? (
                        <EmptyState
                            title="還沒有儲存任何校系"
                            description="在簡章頁面按下「我對此學系有興趣」，就會出現在這裡。"
                            action={
                                <Button asChild size="base">
                                    <Link href="/bochures">瀏覽簡章</Link>
                                </Button>
                            }
                        />
                    ) : (
                        <ul className="flex flex-col gap-4">
                            {programs.map((program) => (
                                <li key={program.program_identifier} className="relative">
                                    <ContentCard
                                        href={programHref(program.program_identifier)}
                                        thumbnailSeed={program.program_identifier}
                                        eyebrow={`${program.academic_year} 學年度`}
                                        title={
                                            <>
                                                {program.school_name}
                                                <span className="block text-lg text-copy-muted sm:text-xl">
                                                    {program.admission_program_name}
                                                </span>
                                            </>
                                        }
                                        meta={
                                            <time dateTime={program.saved_at}>
                                                儲存於 {formatDate(program.saved_at)}
                                            </time>
                                        }
                                    />
                                    <ConfirmDialog
                                        trigger={
                                            <IconButton
                                                label={`移除 ${program.school_name} ${program.admission_program_name}`}
                                                icon={<Trash2 className="size-5" />}
                                                disabled={removing === program.program_identifier}
                                                className="absolute top-3 right-3 border border-ink/15 bg-surface text-ink hover:bg-ink/5"
                                            />
                                        }
                                        title="移除已儲存的校系？"
                                        description={`${program.school_name} ${program.admission_program_name} 將從清單中移除，之後可在簡章頁面重新儲存。`}
                                        confirmLabel="移除"
                                        onConfirm={() => remove(program)}
                                    />
                                </li>
                            ))}
                        </ul>
                    )
                }
            </Resource>
        </AccountPage>
    );
}
