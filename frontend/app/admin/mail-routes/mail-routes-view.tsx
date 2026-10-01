"use client";

import { useEffect, useState } from "react";
import { ChevronDown, ChevronUp, Pencil, Trash2 } from "lucide-react";
import { Popover } from "radix-ui";
import Button from "../../components/button";
import { getMailReplyTemplate, setMailReplyTemplate } from "../../lib/api/admin";
import {
    createMailRoute,
    deleteMailRoute,
    getInquiryDetail,
    listDiscordRoles,
    listMailRouteInquiries,
    listMailRoutes,
    updateMailRoute,
    type DiscordRole,
    type InquiryDetail,
    type InquiryMessage,
    type MailRoute,
    type MailRouteInquiry
} from "../../lib/api/mail-routes";
import { useAdmin } from "../admin-context";
import { AdminPanel, ConfirmDialog, describeError, EmptyState, ErrorText, inputClass, LoadingState, panelClassName } from "../admin-ui";

function RoleMultiSelect({
    roles,
    rolesError,
    selected,
    onToggle,
    label,
    placeholder
}: {
    roles: DiscordRole[];
    rolesError: string | null;
    selected: string[];
    onToggle: (roleID: string) => void;
    label: string;
    placeholder: string;
}) {
    if (rolesError) {
        return (
            <p className="flex flex-col gap-1 text-sm text-ink/50">
                {label}
                <span>目前無法取得身分組清單：{rolesError}</span>
            </p>
        );
    }
    const selectedNames = roles.filter((role) => selected.includes(role.id)).map((role) => role.name);
    return (
        <label className="flex flex-col gap-1 text-sm text-ink/70">
            {label}
            <Popover.Root>
                <Popover.Trigger asChild>
                    <button
                        type="button"
                        disabled={roles.length === 0}
                        className={`${inputClass} flex items-center justify-between gap-2 text-left disabled:opacity-50`}
                    >
                        <span className={`truncate ${selectedNames.length === 0 ? "text-ink/40" : "text-ink"}`}>
                            {roles.length === 0 ? "沒有可選的身分組" : selectedNames.length === 0 ? placeholder : selectedNames.join("、")}
                        </span>
                        <ChevronDown className="h-4 w-4 shrink-0 text-ink/40" aria-hidden />
                    </button>
                </Popover.Trigger>
                <Popover.Portal>
                    <Popover.Content
                        align="start"
                        sideOffset={4}
                        className="z-50 max-h-64 w-56 overflow-y-auto rounded-[var(--radius-small)] border border-ink/15 bg-surface p-1.5 shadow-[var(--shadow-card)]"
                    >
                        {roles.map((role) => (
                            <label
                                key={role.id}
                                className="flex items-center gap-2 rounded-[var(--radius-small)] px-2 py-1.5 text-sm text-ink hover:bg-ink/5"
                            >
                                <input type="checkbox" checked={selected.includes(role.id)} onChange={() => onToggle(role.id)} />
                                {role.name}
                            </label>
                        ))}
                    </Popover.Content>
                </Popover.Portal>
            </Popover.Root>
        </label>
    );
}

function InquiryRow({ inquiry, mfaCode }: { inquiry: MailRouteInquiry; mfaCode: string }) {
    const [open, setOpen] = useState(false);
    const [detail, setDetail] = useState<InquiryDetail | null>(null);
    const [loading, setLoading] = useState(false);
    const [detailError, setDetailError] = useState<string | null>(null);

    async function toggle() {
        if (open) {
            setOpen(false);
            return;
        }
        setOpen(true);
        if (detail) return;
        setLoading(true);
        setDetailError(null);
        try {
            const response = await getInquiryDetail(inquiry.id, mfaCode || undefined);
            setDetail(response.data);
        } catch (cause) {
            setDetailError(describeError(cause));
        } finally {
            setLoading(false);
        }
    }

    return (
        <li className="rounded-[var(--radius-small)] border border-ink/10 bg-surface p-3 text-sm">
            <button type="button" onClick={toggle} className="w-full text-left">
                <div className="flex flex-wrap items-baseline justify-between gap-2">
                    <span className="font-medium text-ink">{inquiry.sender_email}</span>
                    <span className="flex items-center gap-1 text-xs text-ink/50">
                        {new Date(inquiry.created_at).toLocaleString("zh-TW")} ・ 共 {inquiry.message_count} 則往來
                        {open ? <ChevronUp className="h-3.5 w-3.5" aria-hidden /> : <ChevronDown className="h-3.5 w-3.5" aria-hidden />}
                    </span>
                </div>
                {!open && (
                    <p className="mt-1 line-clamp-2 whitespace-pre-line text-ink/70">{inquiry.note}</p>
                )}
            </button>
            {inquiry.discord_thread_id && (
                <p className="mt-1 text-xs text-ink/40">Discord 討論串 ID：{inquiry.discord_thread_id}</p>
            )}
            {open && (
                <div className="mt-3 flex flex-col gap-2 border-t border-ink/10 pt-3">
                    {loading ? (
                        <LoadingState />
                    ) : detailError ? (
                        <ErrorText>{detailError}</ErrorText>
                    ) : (
                        (detail?.messages ?? []).map((message, index) => (
                            <MessageCard key={index} message={message} />
                        ))
                    )}
                </div>
            )}
        </li>
    );
}

// A Gmail-style "show original" panel: the header/body render as normal,
// and every raw-header field (Message-ID, threading headers, source IP,
// auth verdict, self-reported mailer, Discord actor id) sits behind a
// collapsed toggle instead of always taking up space — most messages are
// read for the body, not audited, and the fields that matter for an
// investigation are exactly the ones nobody wants cluttering every row.
function MessageCard({ message }: { message: InquiryMessage }) {
    const [showOriginal, setShowOriginal] = useState(false);
    const hasHeaders = Boolean(
        message.to_address ||
            message.sender_ip ||
            message.auth_results ||
            message.source_message_id ||
            message.in_reply_to ||
            message.references ||
            message.mailer ||
            message.actor_discord_id
    );
    return (
        <div
            className={
                message.direction === "inbound"
                    ? "rounded-[var(--radius-small)] bg-ink/5 p-3"
                    : "rounded-[var(--radius-small)] bg-accent-green/20 p-3"
            }
        >
            <div className="flex flex-wrap items-baseline justify-between gap-2 text-xs text-ink/50">
                <span className="font-medium text-ink/70">
                    {message.direction === "inbound" ? "來信" : "回覆"}
                    {message.actor ? `（${message.actor}）` : ""}
                </span>
                <span>{new Date(message.created_at).toLocaleString("zh-TW")}</span>
            </div>
            {message.subject && <p className="mt-1 text-xs text-ink/50">主旨：{message.subject}</p>}
            <p className="mt-1 whitespace-pre-line text-ink">{message.body}</p>
            {hasHeaders && (
                <div className="mt-2 border-t border-ink/10 pt-2">
                    <button
                        type="button"
                        onClick={() => setShowOriginal((v) => !v)}
                        className="font-sans text-[11px] text-ink/50 underline underline-offset-2 hover:text-ink/70"
                    >
                        {showOriginal ? "隱藏原始資訊" : "顯示原始資訊"}
                    </button>
                    {showOriginal && (
                        <dl className="mt-1.5 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 font-mono text-[11px] text-ink/50">
                            {message.to_address && (
                                <>
                                    <dt className="text-ink/35">收件者</dt>
                                    <dd className="break-all">{message.to_address}</dd>
                                </>
                            )}
                            {message.sender_ip && (
                                <>
                                    <dt className="text-ink/35">來源 IP</dt>
                                    <dd className="break-all">{message.sender_ip}</dd>
                                </>
                            )}
                            {message.auth_results && (
                                <>
                                    <dt className="text-ink/35">驗證結果</dt>
                                    <dd className="break-all">{message.auth_results}</dd>
                                </>
                            )}
                            {message.mailer && (
                                <>
                                    <dt className="text-ink/35">寄件軟體</dt>
                                    <dd className="break-all">{message.mailer}</dd>
                                </>
                            )}
                            {message.source_message_id && (
                                <>
                                    <dt className="text-ink/35">Message-ID</dt>
                                    <dd className="break-all">{message.source_message_id}</dd>
                                </>
                            )}
                            {message.in_reply_to && (
                                <>
                                    <dt className="text-ink/35">In-Reply-To</dt>
                                    <dd className="break-all">{message.in_reply_to}</dd>
                                </>
                            )}
                            {message.references && (
                                <>
                                    <dt className="text-ink/35">References</dt>
                                    <dd className="break-all">{message.references}</dd>
                                </>
                            )}
                            {message.actor_discord_id && (
                                <>
                                    <dt className="text-ink/35">Discord 帳號 ID</dt>
                                    <dd className="break-all">{message.actor_discord_id}</dd>
                                </>
                            )}
                        </dl>
                    )}
                </div>
            )}
        </div>
    );
}

export default function MailRoutesView() {
    const { mfaCode } = useAdmin();
    const [routes, setRoutes] = useState<MailRoute[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);

    const [roles, setRoles] = useState<DiscordRole[]>([]);
    const [rolesError, setRolesError] = useState<string | null>(null);

    const [localPart, setLocalPart] = useState("");
    const [label, setLabel] = useState("");
    const [selectedRoleIDs, setSelectedRoleIDs] = useState<string[]>([]);
    const [routeTemplate, setRouteTemplate] = useState("");
    const [creating, setCreating] = useState(false);
    const [createError, setCreateError] = useState<string | null>(null);

    const [expanded, setExpanded] = useState<string | null>(null);
    const [inquiries, setInquiries] = useState<Record<string, MailRouteInquiry[]>>({});
    const [inquiriesError, setInquiriesError] = useState<Record<string, string>>({});
    const [inquiriesLoading, setInquiriesLoading] = useState<string | null>(null);

    const [editingID, setEditingID] = useState<string | null>(null);
    const [editLabel, setEditLabel] = useState("");
    const [editRoleIDs, setEditRoleIDs] = useState<string[]>([]);
    const [editTemplate, setEditTemplate] = useState("");
    const [editSaving, setEditSaving] = useState(false);
    const [editError, setEditError] = useState<string | null>(null);

    const [replyTemplate, setReplyTemplateState] = useState("");
    const [templateSaving, setTemplateSaving] = useState(false);
    const [templateError, setTemplateError] = useState<string | null>(null);
    const [templateSaved, setTemplateSaved] = useState(false);

    const [deleteTarget, setDeleteTarget] = useState<MailRoute | null>(null);
    const [deleting, setDeleting] = useState(false);

    async function load() {
        setLoading(true);
        setError(null);
        try {
            const response = await listMailRoutes(mfaCode || undefined);
            setRoutes(response.data);
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setLoading(false);
        }
    }

    async function loadRoles() {
        setRolesError(null);
        try {
            const response = await listDiscordRoles(mfaCode || undefined);
            setRoles(response.data);
        } catch (cause) {
            // Discord isn't configured yet, or the guild id is missing — the
            // role picker just stays empty; route creation still works
            // without picking any roles (channel permissions stay untouched).
            setRolesError(describeError(cause));
        }
    }

    useEffect(() => {
        load();
        loadRoles();
        getMailReplyTemplate(mfaCode || undefined)
            .then((response) => setReplyTemplateState(response.value))
            .catch(() => {
                // Leave the textarea empty — saving with an empty template
                // will just fail the [內容]/[簽名] validation, no worse than
                // not being able to load it in the first place.
            });
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [mfaCode]);

    async function handleSaveTemplate() {
        setTemplateSaving(true);
        setTemplateError(null);
        setTemplateSaved(false);
        try {
            const response = await setMailReplyTemplate(replyTemplate, mfaCode || undefined);
            setReplyTemplateState(response.value);
            setTemplateSaved(true);
        } catch (cause) {
            setTemplateError(describeError(cause));
        } finally {
            setTemplateSaving(false);
        }
    }

    function toggleRole(roleID: string) {
        setSelectedRoleIDs((prev) => (prev.includes(roleID) ? prev.filter((id) => id !== roleID) : [...prev, roleID]));
    }

    function toggleEditRole(roleID: string) {
        setEditRoleIDs((prev) => (prev.includes(roleID) ? prev.filter((id) => id !== roleID) : [...prev, roleID]));
    }

    async function handleCreate(event: React.FormEvent) {
        event.preventDefault();
        setCreating(true);
        setCreateError(null);
        try {
            await createMailRoute(
                {
                    local_part: localPart.trim(),
                    label: label.trim(),
                    visible_role_ids: selectedRoleIDs,
                    reply_template: routeTemplate.trim() || null
                },
                mfaCode || undefined
            );
            setLocalPart("");
            setLabel("");
            setSelectedRoleIDs([]);
            setRouteTemplate("");
            await load();
        } catch (cause) {
            setCreateError(describeError(cause));
        } finally {
            setCreating(false);
        }
    }

    async function confirmDelete() {
        if (!deleteTarget) return;
        setDeleting(true);
        try {
            await deleteMailRoute(deleteTarget.id, mfaCode || undefined);
            setDeleteTarget(null);
            await load();
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setDeleting(false);
        }
    }

    function startEdit(route: MailRoute) {
        setEditingID(route.id);
        setEditLabel(route.label);
        setEditRoleIDs(route.visible_role_ids);
        setEditTemplate(route.reply_template ?? "");
        setEditError(null);
    }

    async function handleSaveEdit(route: MailRoute) {
        setEditSaving(true);
        setEditError(null);
        try {
            await updateMailRoute(
                route.id,
                { label: editLabel.trim(), visible_role_ids: editRoleIDs, reply_template: editTemplate.trim() || null },
                mfaCode || undefined
            );
            setEditingID(null);
            await load();
        } catch (cause) {
            setEditError(describeError(cause));
        } finally {
            setEditSaving(false);
        }
    }

    async function toggleExpand(route: MailRoute) {
        if (expanded === route.id) {
            setExpanded(null);
            return;
        }
        setExpanded(route.id);
        if (inquiries[route.id]) return;
        setInquiriesLoading(route.id);
        try {
            const response = await listMailRouteInquiries(route.id, mfaCode || undefined);
            setInquiries((prev) => ({ ...prev, [route.id]: response.data }));
        } catch (cause) {
            setInquiriesError((prev) => ({ ...prev, [route.id]: describeError(cause) }));
        } finally {
            setInquiriesLoading(null);
        }
    }

    return (
        <div className="flex flex-col gap-8">
            <div>
                <h1 className="font-serif text-2xl text-ink">Mail 分類管理</h1>
                <p className="mt-2 font-sans text-sm text-ink/60">
                    每個分類是 mail.sta-tw.org 底下的一個收件地址（例如 brochure@mail.sta-tw.org）。新增分類時會自動在
                    Discord 建立一個對應的論壇頻道，寄到這個地址的信會建立成裡面的貼文。新增後幾秒內即可生效，不需要重啟任何服務。
                </p>
            </div>

            <AdminPanel className="flex flex-col gap-3">
                <div>
                    <h2 className="font-serif text-lg text-ink">預設回信範本</h2>
                    <p className="mt-1 font-sans text-sm text-ink/60">
                        在 Discord 討論串用 /re 回覆時，實際寄出的信件內容會套用這個範本——除非該分類自己設定了專屬範本（見下方「新增分類」與各分類的編輯畫面），那會優先套用分類自己的版本。
                        <code className="mx-1 rounded bg-ink/5 px-1.5 py-0.5 text-ink">[內容]</code>
                        會換成 /re 打的訊息，
                        <code className="mx-1 rounded bg-ink/5 px-1.5 py-0.5 text-ink">[簽名]</code>
                        會換成 /re 的簽名欄位，這兩個必須保留在範本裡。另外還可以選用：
                        <code className="mx-1 rounded bg-ink/5 px-1.5 py-0.5 text-ink">[主旨]</code>
                        原始信件主旨、
                        <code className="mx-1 rounded bg-ink/5 px-1.5 py-0.5 text-ink">[客服人員]</code>
                        回覆的 Discord 顯示名稱、
                        <code className="mx-1 rounded bg-ink/5 px-1.5 py-0.5 text-ink">[收件人]</code>
                        對方的信箱、
                        <code className="mx-1 rounded bg-ink/5 px-1.5 py-0.5 text-ink">[日期]</code>
                        寄出當天的日期，這幾個沒用到也沒關係。
                    </p>
                </div>
                <textarea
                    className={inputClass + " min-h-32 font-mono"}
                    value={replyTemplate}
                    onChange={(e) => {
                        setReplyTemplateState(e.target.value);
                        setTemplateSaved(false);
                    }}
                />
                <div className="flex items-center gap-3">
                    <Button type="button" onClick={handleSaveTemplate} disabled={templateSaving} className="h-9 px-4 text-sm">
                        {templateSaving ? "儲存中…" : "儲存範本"}
                    </Button>
                    {templateSaved && <p className="font-sans text-sm text-green-700">已儲存</p>}
                    {templateError && <ErrorText>{templateError}</ErrorText>}
                </div>
            </AdminPanel>

            <form onSubmit={handleCreate} className={`${panelClassName} flex flex-col gap-4`}>
                <h2 className="font-serif text-lg text-ink">新增分類</h2>
                <div className="flex flex-wrap items-end gap-3">
                    <label className="flex flex-col gap-1 text-sm text-ink/70">
                        收件地址（@ 前面，之後無法更改）
                        <input
                            className={inputClass}
                            value={localPart}
                            onChange={(e) => setLocalPart(e.target.value)}
                            placeholder="brochure"
                            pattern="[a-z0-9][a-z0-9._-]*"
                            required
                        />
                    </label>
                    <label className="flex flex-col gap-1 text-sm text-ink/70">
                        名稱
                        <input
                            className={inputClass}
                            value={label}
                            onChange={(e) => setLabel(e.target.value)}
                            placeholder="簡章回報"
                            required
                        />
                    </label>
                    <div className="min-w-[14rem] flex-1">
                        <RoleMultiSelect
                            roles={roles}
                            rolesError={rolesError}
                            selected={selectedRoleIDs}
                            onToggle={toggleRole}
                            label="誰可以看到這個論壇頻道"
                            placeholder="不勾選則維持頻道現有的權限設定"
                        />
                    </div>
                </div>

                <label className="flex flex-col gap-1 text-sm text-ink/70">
                    這個分類專屬的回信範本（留空則套用上方的預設範本）
                    <textarea
                        className={inputClass + " min-h-24 font-mono"}
                        value={routeTemplate}
                        onChange={(e) => setRouteTemplate(e.target.value)}
                        placeholder="留空＝套用預設範本；要自訂的話必須包含 [內容] 和 [簽名]"
                    />
                </label>

                <div>
                    <Button type="submit" disabled={creating} className="h-10 px-5 text-sm">
                        {creating ? "新增中…" : "新增分類"}
                    </Button>
                    {createError && <ErrorText>{createError}</ErrorText>}
                </div>
            </form>

            {error && <ErrorText>{error}</ErrorText>}
            {loading && !routes ? (
                <LoadingState />
            ) : (
                <div className="flex flex-col gap-3">
                    {(routes ?? []).map((route) => {
                        const isEditing = editingID === route.id;
                        const isExpanded = expanded === route.id;
                        return (
                            <AdminPanel key={route.id} className="p-0">
                                <div className="flex flex-wrap items-center justify-between gap-3 p-4">
                                    <div className="min-w-0">
                                        <p className="font-sans text-sm text-ink/60">{route.local_part}@mail.sta-tw.org</p>
                                        {isEditing ? (
                                            <input
                                                className={inputClass + " mt-1"}
                                                value={editLabel}
                                                onChange={(e) => setEditLabel(e.target.value)}
                                            />
                                        ) : (
                                            <p className="font-serif text-lg text-ink">{route.label}</p>
                                        )}
                                        <p className="mt-1 font-sans text-xs text-ink/40">
                                            Discord 頻道 {route.discord_forum_channel_id} ・ 建立於{" "}
                                            {new Date(route.created_at).toLocaleString("zh-TW")}
                                        </p>
                                    </div>
                                    <div className="flex shrink-0 items-center gap-3">
                                        <button
                                            type="button"
                                            onClick={() => toggleExpand(route)}
                                            className="inline-flex items-center gap-1 font-sans text-sm text-ink/70 hover:text-ink"
                                        >
                                            信件紀錄
                                            {isExpanded ? (
                                                <ChevronUp className="h-4 w-4" aria-hidden />
                                            ) : (
                                                <ChevronDown className="h-4 w-4" aria-hidden />
                                            )}
                                        </button>
                                        {isEditing ? null : (
                                            <button
                                                type="button"
                                                onClick={() => startEdit(route)}
                                                className="inline-flex items-center gap-1 font-sans text-sm text-ink/70 hover:text-ink"
                                            >
                                                <Pencil className="h-4 w-4" aria-hidden />
                                                編輯
                                            </button>
                                        )}
                                        <button
                                            type="button"
                                            onClick={() => setDeleteTarget(route)}
                                            className="inline-flex items-center gap-1 font-sans text-sm text-red-600 hover:underline"
                                        >
                                            <Trash2 className="h-4 w-4" aria-hidden />
                                            刪除
                                        </button>
                                    </div>
                                </div>

                                {isEditing && (
                                    <div className="flex flex-col gap-3 border-t border-ink/10 p-4">
                                        <div className="max-w-xs">
                                            <RoleMultiSelect
                                                roles={roles}
                                                rolesError={rolesError}
                                                selected={editRoleIDs}
                                                onToggle={toggleEditRole}
                                                label="誰可以看到這個論壇頻道"
                                                placeholder="尚未選擇任何身分組"
                                            />
                                        </div>
                                        <label className="flex flex-col gap-1 text-sm text-ink/70">
                                            這個分類專屬的回信範本（留空則套用預設範本）
                                            <textarea
                                                className={inputClass + " min-h-24 font-mono"}
                                                value={editTemplate}
                                                onChange={(e) => setEditTemplate(e.target.value)}
                                                placeholder="留空＝套用預設範本；要自訂的話必須包含 [內容] 和 [簽名]"
                                            />
                                        </label>
                                        <div className="flex items-center gap-3">
                                            <Button
                                                type="button"
                                                onClick={() => handleSaveEdit(route)}
                                                disabled={editSaving}
                                                className="h-9 px-4 text-sm"
                                            >
                                                {editSaving ? "儲存中…" : "儲存"}
                                            </Button>
                                            <button
                                                type="button"
                                                onClick={() => setEditingID(null)}
                                                className="font-sans text-sm text-ink/60 hover:text-ink"
                                            >
                                                取消
                                            </button>
                                            {editError && <ErrorText>{editError}</ErrorText>}
                                        </div>
                                    </div>
                                )}

                                {isExpanded && (
                                    <div className="border-t border-ink/10 p-4">
                                        {inquiriesLoading === route.id ? (
                                            <LoadingState />
                                        ) : inquiriesError[route.id] ? (
                                            <ErrorText>{inquiriesError[route.id]}</ErrorText>
                                        ) : (inquiries[route.id] ?? []).length === 0 ? (
                                            <EmptyState label="還沒有收到任何信件" />
                                        ) : (
                                            <ul className="flex flex-col gap-2">
                                                {(inquiries[route.id] ?? []).map((inquiry) => (
                                                    <InquiryRow key={inquiry.id} inquiry={inquiry} mfaCode={mfaCode || ""} />
                                                ))}
                                            </ul>
                                        )}
                                    </div>
                                )}
                            </AdminPanel>
                        );
                    })}
                    {routes && routes.length === 0 && <EmptyState label="還沒有任何分類" />}
                </div>
            )}

            <ConfirmDialog
                open={deleteTarget !== null}
                onOpenChange={(open) => !open && setDeleteTarget(null)}
                title="刪除這個分類？"
                description={
                    deleteTarget
                        ? `確定要刪除「${deleteTarget.label}」（${deleteTarget.local_part}@mail.sta-tw.org）嗎？刪除後寄到這個地址的信會被拒收。`
                        : undefined
                }
                confirmLabel="刪除"
                pending={deleting}
                onConfirm={() => void confirmDelete()}
            />
        </div>
    );
}
