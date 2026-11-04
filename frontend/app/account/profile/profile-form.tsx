"use client";

import { Plus, Trash2 } from "lucide-react";
import { useState, type FormEvent } from "react";
import Button from "../../components/button";
import { Card } from "../../components/card";
import Checkbox from "../../components/checkbox";
import Field from "../../components/field";
import IconButton from "../../components/icon-button";
import { Input, Textarea } from "../../components/input";
import {
    getMyProfileDetails,
    profileLimits,
    updateMyProfile,
    updateMyProfileDetails,
    type Profile,
    type ProfileDetails,
    type ProfileLink
} from "../../lib/api/profile";
import ConfirmDialog from "../../components/confirm-dialog";
import { useAccount } from "../account-context";
import { ErrorText, Resource, SaveBar } from "../../components/status";
import { useAction, useResource } from "../../lib/use-resource";
import Markdown from "../../components/markdown";
import SchoolPicker, { useSchoolDirectory } from "../../components/school-picker";

const SLUG_PATTERN = /^[a-z0-9_-]{3,30}$/;
const PROFILE_URL_BASE = "https://sta-tw.org/user/";
const LINK_LABEL_MAX = 40;
const LINK_URL_MAX = 300;

type Errors = Partial<Record<"slug" | "links", string>>;

function validateLinks(links: ProfileLink[]): string | undefined {
    for (const link of links) {
        if (!link.url.trim()) continue;
        if (!/^https?:\/\/[^\s/]+/i.test(link.url.trim())) {
            return "連結網址需以 http:// 或 https:// 開頭。";
        }
    }
    return undefined;
}

const loadDetails = (signal: AbortSignal) =>
    getMyProfileDetails({ signal }).then(({ data }) => data);

export default function ProfileForm() {
    const { profileState, reloadProfile } = useAccount();
    const { state, reload } = useResource(loadDetails);
    return (
        <Resource state={profileState} reload={reloadProfile} loadingLabel="正在載入個人資料">
            {(profile) => (
                <Resource state={state} reload={reload} loadingLabel="正在載入個人資料">
                    {(details) => <ProfileFields profile={profile} initialDetails={details} />}
                </Resource>
            )}
        </Resource>
    );
}

function ProfileFields({
    profile,
    initialDetails
}: {
    profile: Profile;
    initialDetails: ProfileDetails;
}) {
    const { me, setProfile } = useAccount();
    const schools = useSchoolDirectory();

    const [displayName, setDisplayName] = useState(profile.display_name ?? "");
    const [bio, setBio] = useState(profile.bio ?? "");
    const [links, setLinks] = useState<ProfileLink[]>(profile.links);
    const [details, setDetails] = useState(initialDetails);
    const [lockedSlug, setLockedSlug] = useState(initialDetails.profile_slug);
    const [slugDraft, setSlugDraft] = useState("");

    const [errors, setErrors] = useState<Errors>({});
    const { pending, error: saveError, run } = useAction();
    const saving = pending !== null;
    const [saved, setSaved] = useState(false);

    /** Wraps a setter so any edit clears the "已儲存" notice. */
    function edited<T>(set: (value: T) => void) {
        return (value: T) => {
            set(value);
            setSaved(false);
        };
    }

    const patchDetails = edited((patch: Partial<ProfileDetails>) =>
        setDetails((current) => ({ ...current, ...patch }))
    );
    const updateLinks = edited(setLinks);

    function patchLink(index: number, patch: Partial<ProfileLink>) {
        updateLinks((current) =>
            current.map((link, i) => (i === index ? { ...link, ...patch } : link))
        );
    }

    const pendingSlug = lockedSlug ? "" : slugDraft.trim();
    // A valid new slug is permanent, so saving goes through a confirm dialog.
    const needsSlugConfirm = SLUG_PATTERN.test(pendingSlug);

    function validate() {
        const next: Errors = {
            links: validateLinks(links),
            slug:
                pendingSlug && !SLUG_PATTERN.test(pendingSlug)
                    ? "自訂網址需為 3–30 個小寫英文字母、數字、- 或 _。"
                    : undefined
        };
        setErrors(next);
        return !next.links && !next.slug;
    }

    function handleSubmit(event: FormEvent) {
        event.preventDefault();
        if (!needsSlugConfirm && validate()) void save();
    }

    async function save() {
        setSaved(false);
        setSaved(await run(saveBoth));
    }

    /** Saves both halves even if one fails, applying whichever succeeded,
     * then rethrows the first failure for useAction to report. */
    async function saveBoth() {
        const [profileResult, detailsResult] = await Promise.allSettled([
            updateMyProfile({
                display_name: displayName,
                bio,
                links: links.filter((link) => link.url.trim())
            }),
            updateMyProfileDetails({
                ...details,
                school_name: details.school_name.trim(),
                department: details.department.trim(),
                profile_slug: lockedSlug ?? (pendingSlug || null)
            })
        ]);
        if (profileResult.status === "fulfilled") {
            setProfile(profileResult.value.data);
            setLinks(profileResult.value.data.links);
        }
        if (detailsResult.status === "fulfilled") {
            setLockedSlug(detailsResult.value.data.profile_slug);
        }
        const failure = [profileResult, detailsResult].find(
            (result) => result.status === "rejected"
        );
        if (failure) throw failure.reason;
    }

    return (
        <form onSubmit={handleSubmit} className="flex flex-col gap-6" noValidate>
            <Card title="基本資料" className="flex flex-col gap-6">
                <Field label="帳號" hint="帳號無法變更。">
                    <Input value={me.account.username} readOnly />
                </Field>

                <div className="flex flex-col gap-3">
                    <Field
                        label="就讀學校"
                        error={schools.error ? `學校清單載入失敗：${schools.error}` : undefined}
                    >
                        <SchoolPicker
                            directory={schools}
                            value={details.school_name}
                            onChange={(school_name) => patchDetails({ school_name })}
                        />
                    </Field>
                    <Checkbox
                        label="不要在個人檔案頁面顯示我的學校"
                        checked={details.hide_school}
                        onCheckedChange={(hide_school) => patchDetails({ hide_school })}
                    />
                </div>

                <div className="flex flex-col gap-3">
                    <Field label="系級">
                        <Input
                            value={details.department}
                            onChange={(event) => patchDetails({ department: event.target.value })}
                            placeholder="請填寫全名，如：資訊工程學系三年級、普通科一年級"
                            maxLength={80}
                        />
                    </Field>
                    <Checkbox
                        label="不要在個人檔案頁面顯示我的系級"
                        checked={details.hide_department}
                        onCheckedChange={(hide_department) => patchDetails({ hide_department })}
                    />
                </div>

                <Field
                    label="暱稱（公開顯示）"
                    hint={`${Array.from(displayName).length} / ${profileLimits.displayName}`}
                >
                    <Input
                        value={displayName}
                        onChange={(event) => edited(setDisplayName)(event.target.value)}
                        maxLength={profileLimits.displayName}
                        placeholder={me.account.username}
                    />
                </Field>

                <div className="grid gap-4 lg:grid-cols-2">
                    <Field
                        label="個人簡介"
                        hint={`支援 Markdown（**粗體**、*斜體*、清單、[連結](https://…)）。${Array.from(bio).length} / ${profileLimits.bio}`}
                    >
                        <Textarea
                            value={bio}
                            onChange={(event) => edited(setBio)(event.target.value)}
                            maxLength={profileLimits.bio}
                            rows={8}
                        />
                    </Field>
                    <section aria-label="個人簡介預覽" className="flex min-w-0 flex-col gap-2">
                        <h3 className="font-sans text-base font-medium text-ink">預覽</h3>
                        <div className="min-h-32 flex-1 rounded-[var(--radius-control)] border border-ink/10 bg-ink/[0.03] p-4">
                            {bio.trim() ? (
                                <Markdown markdown={bio} />
                            ) : (
                                <p className="font-sans text-sm text-copy-muted">
                                    尚未填寫個人簡介。
                                </p>
                            )}
                        </div>
                    </section>
                </div>
            </Card>

            <Card
                title="個人連結"
                description={`最多 ${profileLimits.links} 個，例如作品集、GitHub 或個人網站。`}
                className="flex flex-col gap-4"
            >
                {links.length > 0 && (
                    <ul className="flex flex-col gap-4">
                        {links.map((link, index) => (
                            <li
                                key={index}
                                className="grid items-end gap-3 sm:grid-cols-[minmax(0,12rem)_minmax(0,1fr)_auto]"
                            >
                                <Field label={`連結 ${index + 1} 名稱`}>
                                    <Input
                                        value={link.label}
                                        onChange={(event) =>
                                            patchLink(index, { label: event.target.value })
                                        }
                                        maxLength={LINK_LABEL_MAX}
                                        placeholder="例如：GitHub"
                                    />
                                </Field>
                                <Field label={`連結 ${index + 1} 網址`}>
                                    <Input
                                        type="url"
                                        inputMode="url"
                                        value={link.url}
                                        onChange={(event) =>
                                            patchLink(index, { url: event.target.value })
                                        }
                                        maxLength={LINK_URL_MAX}
                                        placeholder="https://"
                                    />
                                </Field>
                                <IconButton
                                    type="button"
                                    label={`移除連結 ${index + 1}`}
                                    icon={<Trash2 className="size-5" />}
                                    className="text-copy-muted hover:bg-ink/5 hover:text-danger"
                                    onClick={() =>
                                        updateLinks((current) =>
                                            current.filter((_, i) => i !== index)
                                        )
                                    }
                                />
                            </li>
                        ))}
                    </ul>
                )}
                <ErrorText>{errors.links}</ErrorText>
                {links.length < profileLimits.links && (
                    <Button
                        type="button"
                        variant="secondary"
                        size="base"
                        className="gap-2 self-start"
                        onClick={() =>
                            updateLinks((current) => [...current, { label: "", url: "" }])
                        }
                    >
                        <Plus aria-hidden className="size-4" />
                        新增連結
                    </Button>
                )}
            </Card>

            <Card title="自訂個人網址" className="flex flex-col gap-4">
                {lockedSlug ? (
                    <Field label="個人網址" hint="自訂網址設定後就無法更改。">
                        <Input value={`${PROFILE_URL_BASE}${lockedSlug}`} readOnly />
                    </Field>
                ) : (
                    <Field
                        label="網址代稱"
                        hint={`3–30 個小寫英文字母、數字、- 或 _。設定後無法更改。預覽：${PROFILE_URL_BASE}${pendingSlug || "…"}`}
                        error={errors.slug}
                    >
                        <Input
                            value={slugDraft}
                            onChange={(event) =>
                                edited(setSlugDraft)(event.target.value.toLowerCase())
                            }
                            maxLength={30}
                            autoCapitalize="none"
                            autoComplete="off"
                            spellCheck={false}
                            placeholder="例如：my-name"
                        />
                    </Field>
                )}
            </Card>

            <SaveBar pending={saving} saved={saved} error={saveError}>
                {needsSlugConfirm && (
                    <ConfirmDialog
                        trigger={
                            <Button type="button" size="base" disabled={saving}>
                                {saving ? "儲存中…" : "儲存"}
                            </Button>
                        }
                        tone="primary"
                        title="確定使用這個網址嗎？"
                        description={
                            <>
                                你的個人網址將設定為{" "}
                                <span className="font-medium break-all text-ink">
                                    {PROFILE_URL_BASE}
                                    {pendingSlug}
                                </span>
                                ，設定後無法更改。
                            </>
                        }
                        confirmLabel="確定並儲存"
                        onConfirm={() => {
                            if (validate()) void save();
                        }}
                    />
                )}
            </SaveBar>
        </form>
    );
}
