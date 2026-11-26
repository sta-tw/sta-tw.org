"use client";

import {
    closestCenter,
    DndContext,
    KeyboardSensor,
    PointerSensor,
    useSensor,
    useSensors,
    type DragEndEvent
} from "@dnd-kit/core";
import {
    arrayMove,
    SortableContext,
    sortableKeyboardCoordinates,
    useSortable,
    verticalListSortingStrategy
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { GripVertical, Pencil, X } from "lucide-react";
import { useCallback, useState } from "react";
import { twMerge } from "tailwind-merge";
import Button from "../../components/button";
import IconButton from "../../components/icon-button";
import { Input } from "../../components/input";
import Select from "../../components/select";
import { ErrorText, StatusText } from "../../components/status";
import {
    getApplicationResult,
    listApplicationInquiries,
    setApplicationPreferences,
    setCandidateNumber,
    setWillingness,
    WILLINGNESS_VALUES,
    willingnessLabels,
    type Application
} from "../../lib/api/applications";
import { ApiError } from "../../lib/api/types";
import { formatDateTime } from "../../lib/format";
import { useAction, useResource } from "../../lib/use-resource";

const willingnessOptions = [...WILLINGNESS_VALUES].reverse().map((value) => ({
    value: String(value),
    label: `${willingnessLabels[value]}（${value}）`
}));

const rowClass =
    "flex items-stretch gap-3 rounded-[var(--radius-panel)] border border-ink/10 bg-surface p-3 sm:gap-4 sm:p-4";

const badgeClass =
    "flex w-12 shrink-0 items-center justify-center rounded-[var(--radius-control)] font-sans font-medium text-ink";

const nameOf = (application: Application) =>
    `${application.school_name} ${application.program_name}`;

/** 報考校系清單及志願序: applications with a candidate number are ranked
 * and can be dragged (or moved with the keyboard) into preference order;
 * the rest wait for their number. */
export default function PreferenceList({
    applications,
    onChange
}: {
    applications: Application[];
    onChange: (applications: Application[]) => void;
}) {
    const ranked = applications
        .filter((item) => item.preference_rank !== undefined)
        .sort((a, b) => a.preference_rank! - b.preference_rank!);
    const unranked = applications.filter((item) => item.preference_rank === undefined);
    const { error, run } = useAction();

    const sensors = useSensors(
        useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
        useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
    );

    /** Applies a new order locally right away; reverts if saving fails. */
    function reorder(next: Application[]) {
        const previous = applications;
        const ranks = new Map(next.map((item, index) => [item.id, index + 1]));
        onChange(
            applications.map((item) =>
                ranks.has(item.id) ? { ...item, preference_rank: ranks.get(item.id) } : item
            )
        );
        void run("order", async () => {
            try {
                await setApplicationPreferences(next.map((item) => item.id));
            } catch (cause) {
                onChange(previous);
                throw cause;
            }
        });
    }

    function handleDragEnd({ active, over }: DragEndEvent) {
        if (!over || active.id === over.id) return;
        const from = ranked.findIndex((item) => item.id === active.id);
        const to = ranked.findIndex((item) => item.id === over.id);
        if (from !== -1 && to !== -1) reorder(arrayMove(ranked, from, to));
    }

    function numberSaved(application: Application, last4: string) {
        const wasRanked = application.preference_rank !== undefined;
        onChange(
            applications.map((item) =>
                item.id === application.id
                    ? {
                          ...item,
                          candidate_number_last4: last4,
                          preference_rank: wasRanked ? item.preference_rank : ranked.length + 1
                      }
                    : item
            )
        );
        if (!wasRanked) {
            // listApplications ranks unordered applications last anyway, so
            // a failed save here loses nothing.
            setApplicationPreferences([...ranked, application].map((item) => item.id)).catch(
                () => undefined
            );
        }
    }

    return (
        <div className="flex flex-col gap-3">
            {ranked.length > 1 && (
                <p id="preference-drag-hint" className="font-sans text-sm text-copy-muted">
                    拖曳左側把手調整志願序；使用鍵盤時，聚焦把手後按空白鍵拿起，再用方向鍵移動。
                </p>
            )}
            <ErrorText>{error}</ErrorText>
            <DndContext
                sensors={sensors}
                collisionDetection={closestCenter}
                onDragEnd={handleDragEnd}
            >
                <SortableContext
                    items={ranked.map((item) => item.id)}
                    strategy={verticalListSortingStrategy}
                >
                    <ol aria-label="志願序" className="flex flex-col gap-3">
                        {ranked.map((application, index) => (
                            <RankedRow
                                key={application.id}
                                application={application}
                                rank={index + 1}
                                sortable={ranked.length > 1}
                                onNumberSaved={(last4) => numberSaved(application, last4)}
                            />
                        ))}
                    </ol>
                </SortableContext>
            </DndContext>
            {unranked.length > 0 && (
                <ul aria-label="尚未填寫准考證號碼" className="flex flex-col gap-3">
                    {unranked.map((application) => (
                        <li key={application.id} className={rowClass}>
                            <span
                                className={twMerge(
                                    badgeClass,
                                    "bg-accent-yellow text-sm leading-tight"
                                )}
                            >
                                待填
                            </span>
                            <div className="flex min-w-0 flex-1 flex-col gap-3">
                                <ProgramName application={application} />
                                <CandidateNumberForm
                                    application={application}
                                    onSaved={(last4) => numberSaved(application, last4)}
                                />
                            </div>
                        </li>
                    ))}
                </ul>
            )}
        </div>
    );
}

function ProgramName({ application }: { application: Application }) {
    return (
        <div className="min-w-0">
            <p className="font-sans text-sm text-copy-muted">{application.academic_year} 學年度</p>
            <p className="font-sans text-lg leading-snug font-medium break-words text-ink">
                {nameOf(application)}
            </p>
        </div>
    );
}

function RankedRow({
    application,
    rank,
    sortable,
    onNumberSaved
}: {
    application: Application;
    rank: number;
    sortable: boolean;
    onNumberSaved: (last4: string) => void;
}) {
    const {
        attributes,
        listeners,
        setNodeRef,
        setActivatorNodeRef,
        transform,
        transition,
        isDragging
    } = useSortable({ id: application.id, disabled: !sortable });
    const [editing, setEditing] = useState(false);

    return (
        <li
            ref={setNodeRef}
            style={{ transform: CSS.Transform.toString(transform), transition }}
            className={twMerge(rowClass, isDragging && "relative z-10 shadow-[var(--shadow-card)]")}
        >
            {sortable && (
                <button
                    ref={setActivatorNodeRef}
                    type="button"
                    {...attributes}
                    {...listeners}
                    aria-label={`調整「${nameOf(application)}」的志願序，目前第 ${rank} 志願`}
                    aria-describedby="preference-drag-hint"
                    className="-ml-1 flex w-6 shrink-0 cursor-grab touch-none items-center justify-center rounded-[var(--radius-small)] text-copy-muted focus-ring hover:text-ink active:cursor-grabbing"
                >
                    <GripVertical aria-hidden className="size-5" />
                </button>
            )}
            <span
                aria-label={`第 ${rank} 志願`}
                className={twMerge(badgeClass, "bg-accent-green text-xl")}
            >
                {rank}
            </span>
            <div className="flex min-w-0 flex-1 flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                <div className="min-w-0">
                    <ProgramName application={application} />
                    <p className="mt-1 flex items-center gap-1 font-sans text-sm text-copy-muted">
                        准考證號碼：末四碼 {application.candidate_number_last4}
                        <IconButton
                            label={editing ? "取消修改准考證號碼" : "修改准考證號碼"}
                            icon={
                                editing ? <X className="size-4" /> : <Pencil className="size-4" />
                            }
                            className="size-8 hover:bg-ink/5 hover:text-ink"
                            onClick={() => setEditing((value) => !value)}
                        />
                    </p>
                    {editing && (
                        <div className="mt-2">
                            <CandidateNumberForm
                                application={application}
                                onSaved={(last4) => {
                                    setEditing(false);
                                    onNumberSaved(last4);
                                }}
                            />
                        </div>
                    )}
                </div>
                <WillingnessControl applicationID={application.id} />
            </div>
        </li>
    );
}

function CandidateNumberForm({
    application,
    onSaved
}: {
    application: Application;
    onSaved: (last4: string) => void;
}) {
    const [value, setValue] = useState("");
    const { pending, error, run } = useAction();
    const trimmed = value.trim();

    async function submit(event: React.FormEvent) {
        event.preventDefault();
        if (await run(() => setCandidateNumber(application.id, trimmed))) {
            setValue("");
            onSaved(trimmed.slice(-4));
        }
    }

    return (
        <form onSubmit={submit} className="flex flex-col gap-2">
            <div className="flex gap-2">
                <Input
                    value={value}
                    aria-label={`${nameOf(application)} 的准考證號碼`}
                    placeholder="輸入准考證號碼"
                    autoComplete="off"
                    minLength={4}
                    maxLength={64}
                    pattern="[A-Za-z0-9_\-]{4,64}"
                    onChange={(event) => setValue(event.target.value)}
                />
                <Button
                    type="submit"
                    variant="secondary"
                    size="base"
                    className="shrink-0 px-5"
                    disabled={pending !== null || trimmed.length < 4}
                >
                    {pending ? "儲存中…" : "儲存"}
                </Button>
            </div>
            <ErrorText>{error}</ErrorText>
        </form>
    );
}

/** 404 means "not available yet" for both the result and the inquiries. */
const notFoundAs =
    <T,>(fallback: T) =>
    (cause: unknown): T => {
        if (cause instanceof ApiError && cause.status === 404) return fallback;
        throw cause;
    };

/** 報到意願 for one application; it can only be given once the official
 * list is out and the student is admitted or waitlisted. */
function WillingnessControl({ applicationID }: { applicationID: string }) {
    const load = useCallback(
        async (signal: AbortSignal) => {
            const [result, inquiries] = await Promise.all([
                getApplicationResult(applicationID, { signal }).then(
                    ({ data }) => data,
                    notFoundAs(null)
                ),
                listApplicationInquiries(applicationID, { signal }).then(
                    ({ data }) => data,
                    notFoundAs([])
                )
            ]);
            return { result, inquiries };
        },
        [applicationID]
    );
    const { state, setData } = useResource(load);
    const { pending, error, run } = useAction();
    const [saved, setSaved] = useState(false);

    const ready = state.status === "ready" ? state.data : null;
    const canRespond =
        ready?.result?.result_status === "admitted" ||
        ready?.result?.result_status === "waitlisted";
    const openInquiry = ready?.inquiries
        .filter((inquiry) => !inquiry.responded)
        .sort((a, b) => b.created_at.localeCompare(a.created_at))[0];

    function change(next: string) {
        if (!ready?.result || next === "") return;
        const value = Number(next) as (typeof WILLINGNESS_VALUES)[number];
        setSaved(false);
        void run(async () => {
            await setWillingness(applicationID, { value, inquiry_id: openInquiry?.id });
            setData((data) => ({
                ...data,
                result: data.result && { ...data.result, current_willingness: value }
            }));
            setSaved(true);
        });
    }

    const current = ready?.result?.current_willingness;

    return (
        <div className="flex w-full flex-col gap-1 lg:w-56 lg:shrink-0">
            <Select
                aria-label="報到意願"
                value={current === undefined ? "" : String(current)}
                placeholder={
                    state.status === "loading"
                        ? "載入中…"
                        : canRespond
                          ? "請選擇報到意願"
                          : "放榜後開放填寫"
                }
                options={willingnessOptions}
                disabled={!canRespond || pending !== null}
                onChange={(event) => change(event.target.value)}
                className={twMerge(
                    canRespond && current === undefined && "border-accent-yellow-deep"
                )}
            />
            {openInquiry?.response_deadline && (
                <p className="font-sans text-xs text-copy-muted">
                    請於 {formatDateTime(openInquiry.response_deadline)} 前回覆
                </p>
            )}
            <StatusText>{saved ? "已更新報到意願。" : null}</StatusText>
            <ErrorText>{error}</ErrorText>
        </div>
    );
}
