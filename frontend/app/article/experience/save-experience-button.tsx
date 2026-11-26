"use client";

import { Bookmark } from "lucide-react";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { twMerge } from "tailwind-merge";
import ToggleChip from "../../components/toggle-chip";
import {
    getSavedExperienceStatus,
    saveExperience,
    unsaveExperience,
    type TaggedExperience
} from "../../lib/api/content";
import { useCurrentAccount } from "../../lib/current-account";
import { loginHref } from "../../lib/routes";
import { useAction } from "../../lib/use-resource";
import { ErrorText } from "../../components/status";

/** 儲存文章 toggle; saved articles are listed under 已儲存的項目. Signed-out
 * readers are sent to log in first. */
export default function SaveExperienceButton({ experience }: { experience: TaggedExperience }) {
    const { status } = useCurrentAccount();
    const router = useRouter();
    const pathname = usePathname();
    const [saved, setSaved] = useState<{ id: string; value: boolean } | null>(null);
    const { pending, error, run } = useAction();
    const signedIn = status === "authenticated";

    useEffect(() => {
        if (!signedIn) return;
        const controller = new AbortController();
        getSavedExperienceStatus(experience.id, { signal: controller.signal }).then(
            ({ data }) => setSaved({ id: experience.id, value: data.saved }),
            // Unknown state just shows the button as unsaved.
            () => undefined
        );
        return () => controller.abort();
    }, [signedIn, experience.id]);

    const isSaved = saved?.id === experience.id && saved.value;

    return (
        <div className="flex flex-col items-end gap-1">
            <ToggleChip
                tone="yellow"
                pressed={isSaved}
                disabled={pending !== null}
                onPressedChange={(next) => {
                    if (!signedIn) {
                        router.push(loginHref(`${pathname}?id=${experience.id}`));
                        return;
                    }
                    void run(async () => {
                        await (next ? saveExperience(experience) : unsaveExperience(experience.id));
                        setSaved({ id: experience.id, value: next });
                    });
                }}
                className="gap-2"
            >
                <Bookmark aria-hidden className={twMerge("size-4", isSaved && "fill-ink")} />
                {isSaved ? "已儲存" : "儲存文章"}
            </ToggleChip>
            <ErrorText>{error}</ErrorText>
        </div>
    );
}
