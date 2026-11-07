"use client";

import { Camera } from "lucide-react";
import { useState } from "react";
import Avatar from "../../components/avatar";
import Button from "../../components/button";
import { Card } from "../../components/card";
import Field from "../../components/field";
import FileUpload from "../../components/file-upload";
import { Resource, StatusText } from "../../components/status";
import {
    AVATAR_ACCEPT,
    AVATAR_MAX_BYTES,
    deleteAvatar,
    displayNameOf,
    profileAvatarSrc,
    uploadAvatar,
    type Profile
} from "../../lib/api/profile";
import { useAction } from "../../lib/use-resource";
import { useAccount } from "../account-context";
import { cropToSquare } from "./crop-to-square";

export default function AvatarSection() {
    const { profileState, reloadProfile } = useAccount();
    return (
        <Card title="大頭貼" className="flex flex-col gap-5">
            <Resource state={profileState} reload={reloadProfile} loadingLabel="正在載入大頭貼">
                {(profile) => <AvatarEditor profile={profile} />}
            </Resource>
        </Card>
    );
}

function AvatarEditor({ profile }: { profile: Profile }) {
    const { setProfile } = useAccount();
    const { pending, error, setError, run } = useAction();
    const [status, setStatus] = useState<string | null>(null);
    const busy = pending !== null;
    const name = displayNameOf(profile);

    async function handleFile(file: File) {
        setStatus(null);
        if (!AVATAR_ACCEPT.split(",").includes(file.type)) {
            setError("請上傳 PNG 或 JPEG 圖片。");
            return;
        }
        if (file.size > AVATAR_MAX_BYTES) {
            setError("圖片需小於 2 MB。");
            return;
        }
        let problem: string | null = null;
        const ok = await run(async () => {
            const square = await cropToSquare(file).catch(() => null);
            if (!square) problem = "無法讀取這張圖片，請換一張試試。";
            else if (square.size > AVATAR_MAX_BYTES) {
                problem = "裁切後的圖片仍超過 2 MB，請換一張較小的圖片。";
            } else {
                const { data } = await uploadAvatar(square);
                setProfile(data);
            }
        });
        if (problem) setError(problem);
        else if (ok) setStatus("大頭貼已更新。");
    }

    async function handleRemove() {
        setStatus(null);
        const ok = await run(async () => {
            await deleteAvatar();
            setProfile({ ...profile, has_avatar: false, avatar_updated_at: undefined });
        });
        if (ok) setStatus("已移除大頭貼。");
    }

    return (
        <div className="flex flex-col items-start gap-6 sm:flex-row sm:items-center">
            <div className="relative">
                <Avatar
                    name={name}
                    src={profileAvatarSrc(profile)}
                    size="xl"
                    className="size-28 sm:size-36"
                />
                <span
                    aria-hidden
                    className="absolute right-1 bottom-1 inline-flex size-9 items-center justify-center rounded-full border border-ink/10 bg-surface text-ink shadow-[var(--shadow-card)]"
                >
                    <Camera className="size-5" />
                </span>
            </div>
            <div className="flex min-w-0 flex-col gap-3">
                <Field
                    label={<span className="sr-only">大頭貼圖片</span>}
                    hint="圖片比例為 1:1（非正方形會自動置中裁切），PNG 或 JPEG，2 MB 以內。"
                    error={error}
                >
                    <FileUpload
                        accept={AVATAR_ACCEPT}
                        label={busy ? "處理中…" : "上傳大頭貼"}
                        disabled={busy}
                        onFileSelected={handleFile}
                    />
                </Field>
                {profile.has_avatar && (
                    <Button
                        type="button"
                        variant="secondary"
                        size="base"
                        className="self-start"
                        disabled={busy}
                        onClick={handleRemove}
                    >
                        移除大頭貼
                    </Button>
                )}
                <StatusText>{status}</StatusText>
            </div>
        </div>
    );
}
