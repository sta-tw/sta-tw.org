"use client";

import Link from "next/link";
import { CalendarClock, Trash2 } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import Button from "../../components/button";
import ConfirmDialog from "../../components/confirm-dialog";
import ExperienceCard from "../../components/experience-card";
import IconButton from "../../components/icon-button";
import PortfolioCard from "../../components/portfolio-card";
import { EmptyState, ErrorText, Resource } from "../../components/status";
import { nextTimelineMilestone } from "../../lib/admissions-brochures";
import { getAdmissionProgram } from "../../lib/api/admissions";
import { listSavedExperiences, unsaveExperience } from "../../lib/api/content";
import { listSavedPortfolios } from "../../lib/api/portfolio";
import { listSavedPrograms, unsaveProgram, type SavedProgram } from "../../lib/api/saved-programs";
import { programHref } from "../../lib/routes";
import { useAction, useResource } from "../../lib/use-resource";
import AccountPage from "../account-page";

const loadSaved = (signal: AbortSignal) =>
    Promise.all([
        listSavedPrograms({ signal }),
        listSavedPortfolios({ signal }),
        listSavedExperiences({ signal })
    ]).then(([programs, portfolios, experiences]) => ({
        programs: programs.data,
        portfolios: portfolios.data,
        experiences: experiences.data
    }));

export default function SavedView() {
    const { state, reload, setData } = useResource(loadSaved);
    const { pending, error, run } = useAction();

    const removeProgram = (program: SavedProgram) =>
        run(program.program_identifier, async () => {
            await unsaveProgram(program.program_identifier);
            setData((data) => ({
                ...data,
                programs: data.programs.filter(
                    (item) => item.program_identifier !== program.program_identifier
                )
            }));
        });

    const removeExperience = (id: string) =>
        run(id, async () => {
            await unsaveExperience(id);
            setData((data) => ({
                ...data,
                experiences: data.experiences.filter((item) => item.id !== id)
            }));
        });

    return (
        <AccountPage title="已儲存的項目" description="你收藏的簡章、備審與文章。">
            <ErrorText>{error}</ErrorText>
            <Resource state={state} reload={reload}>
                {({ programs, portfolios, experiences }) =>
                    programs.length + portfolios.length + experiences.length === 0 ? (
                        <EmptyState
                            title="還沒有儲存任何項目"
                            description="在簡章頁按下「我對此學系有興趣」，或在備審、文章旁按下儲存，就會出現在這裡。"
                            action={
                                <Button asChild size="base">
                                    <Link href="/bochures">瀏覽簡章</Link>
                                </Button>
                            }
                        />
                    ) : (
                        <>
                            <SavedSection
                                id="programs"
                                title="簡章"
                                empty={
                                    <>
                                        在簡章頁按下「我對此學系有興趣」即可收藏。
                                        <Link href="/bochures" className={inlineLinkClass}>
                                            瀏覽簡章
                                        </Link>
                                    </>
                                }
                            >
                                {programs.map((program) => (
                                    <li key={program.program_identifier}>
                                        <SavedProgramRow
                                            program={program}
                                            busy={pending === program.program_identifier}
                                            onRemove={() => removeProgram(program)}
                                        />
                                    </li>
                                ))}
                            </SavedSection>
                            <SavedSection
                                id="portfolios"
                                title="交換／公開備審"
                                empty={
                                    <>
                                        按下備審旁的書籤即可收藏。
                                        <Link href="/portfolio" className={inlineLinkClass}>
                                            看看備審
                                        </Link>
                                    </>
                                }
                            >
                                {portfolios.map((portfolio) => (
                                    <li key={portfolio.id}>
                                        <PortfolioCard
                                            portfolio={portfolio}
                                            onSavedChange={(saved) => {
                                                if (!saved) {
                                                    setData((data) => ({
                                                        ...data,
                                                        portfolios: data.portfolios.filter(
                                                            (item) => item.id !== portfolio.id
                                                        )
                                                    }));
                                                }
                                            }}
                                        />
                                    </li>
                                ))}
                            </SavedSection>
                            <SavedSection
                                id="experiences"
                                title="文章"
                                empty={
                                    <>
                                        在文章頁按下「儲存文章」即可收藏。
                                        <Link href="/articlemain" className={inlineLinkClass}>
                                            逛逛文章
                                        </Link>
                                    </>
                                }
                            >
                                {experiences.map((experience) => (
                                    <li key={experience.id} className="relative">
                                        <ExperienceCard experience={experience} />
                                        <IconButton
                                            label={`取消儲存「${experience.title}」`}
                                            icon={<Trash2 className="size-5" />}
                                            disabled={pending === experience.id}
                                            onClick={() => removeExperience(experience.id)}
                                            className="absolute top-3 right-3 size-10 border border-ink/15 bg-surface text-ink hover:bg-ink/5"
                                        />
                                    </li>
                                ))}
                            </SavedSection>
                        </>
                    )
                }
            </Resource>
        </AccountPage>
    );
}

const inlineLinkClass = "ml-1 text-ink underline underline-offset-4";

function SavedSection({
    id,
    title,
    empty,
    children
}: {
    id: string;
    title: string;
    empty: ReactNode;
    children: ReactNode[];
}) {
    return (
        <section
            aria-labelledby={`saved-${id}`}
            className="flex flex-col gap-3 border-t border-ink/10 pt-6 first:border-t-0 first:pt-0"
        >
            <h2 id={`saved-${id}`} className="font-sans text-2xl font-medium text-ink">
                {title}
                {children.length > 0 && (
                    <span className="ml-2 font-sans text-base font-normal text-copy-muted">
                        {children.length}
                    </span>
                )}
            </h2>
            {children.length === 0 ? (
                <p className="font-sans text-base text-copy-muted">{empty}</p>
            ) : (
                <ul className="flex flex-col gap-3">{children}</ul>
            )}
        </section>
    );
}

function SavedProgramRow({
    program,
    busy,
    onRemove
}: {
    program: SavedProgram;
    busy: boolean;
    onRemove: () => void;
}) {
    const milestone = useNextMilestone(program.program_identifier);
    const name = `${program.school_name} ${program.admission_program_name}`;

    return (
        <div className="flex items-center gap-2 rounded-[var(--radius-panel)] border border-ink/10 bg-surface pr-2 transition-colors hover:border-ink/25">
            <Link
                href={programHref(program.program_identifier)}
                className="min-w-0 flex-1 rounded-[var(--radius-panel)] p-4 focus-ring sm:px-5"
            >
                <p className="font-sans text-sm text-copy-muted">{program.academic_year} 學年度</p>
                <p className="font-sans text-lg leading-snug font-medium break-words text-ink">
                    {name}
                </p>
                <p className="mt-1 flex items-start gap-1.5 font-sans text-sm text-copy-muted">
                    <CalendarClock aria-hidden className="mt-0.5 size-4 shrink-0" />
                    {milestone === undefined
                        ? "正在載入時程…"
                        : milestone
                          ? `下一個時程：${milestone.title}（${milestone.date}）`
                          : "目前沒有即將到來的時程"}
                </p>
            </Link>
            <ConfirmDialog
                trigger={
                    <IconButton
                        label={`移除 ${name}`}
                        icon={<Trash2 className="size-5" />}
                        disabled={busy}
                        className="size-10 shrink-0 text-copy-muted hover:bg-ink/5 hover:text-ink"
                    />
                }
                title="移除已儲存的校系？"
                description={`${name} 將從清單中移除，之後可在簡章頁面重新儲存。`}
                confirmLabel="移除"
                onConfirm={onRemove}
            />
        </div>
    );
}

/** undefined while loading; null when there's nothing upcoming (or the
 * schedule couldn't be loaded — it's secondary information). */
function useNextMilestone(identifier: string) {
    const [entry, setEntry] = useState<{
        identifier: string;
        milestone: ReturnType<typeof nextTimelineMilestone>;
    } | null>(null);
    useEffect(() => {
        const controller = new AbortController();
        getAdmissionProgram(identifier, { signal: controller.signal })
            .then(
                ({ data }) => nextTimelineMilestone(data),
                () => null
            )
            .then((milestone) => {
                if (!controller.signal.aborted) setEntry({ identifier, milestone });
            });
        return () => controller.abort();
    }, [identifier]);
    return entry?.identifier === identifier ? entry.milestone : undefined;
}
