"use client";

import { Label, RadioGroup } from "radix-ui";
import { useId, useState, type FormEvent } from "react";
import { Card } from "../../components/card";
import Checkbox from "../../components/checkbox";
import {
    getPrivacySettings,
    updatePrivacySettings,
    type PrivacySettings
} from "../../lib/api/profile";
import { Resource, SaveBar } from "../../components/status";
import { useAction, useResource } from "../../lib/use-resource";

const visibilityOptions: {
    value: PrivacySettings["profile_visibility"];
    label: string;
    description: string;
}[] = [
    { value: "public", label: "公開", description: "任何人都能看到你的個人檔案頁面。" },
    { value: "members", label: "僅登入會員", description: "只有登入的會員能看到你的個人檔案頁面。" }
];

const loadSettings = (signal: AbortSignal) =>
    getPrivacySettings({ signal }).then(({ data }) => data);

export default function PrivacyForm() {
    const { state, reload } = useResource(loadSettings);
    return (
        <Resource state={state} reload={reload} loadingLabel="正在載入隱私權設定">
            {(initial) => <PrivacyFields initial={initial} />}
        </Resource>
    );
}

function PrivacyFields({ initial }: { initial: PrivacySettings }) {
    const [settings, setSettings] = useState(initial);
    const { pending, error, run } = useAction();
    const [saved, setSaved] = useState(false);
    const visibilityId = useId();

    function patch(next: Partial<PrivacySettings>) {
        setSettings((current) => ({ ...current, ...next }));
        setSaved(false);
    }

    async function handleSubmit(event: FormEvent) {
        event.preventDefault();
        setSaved(
            await run(async () => {
                const { data } = await updatePrivacySettings(settings);
                setSettings(data);
            })
        );
    }

    return (
        <form onSubmit={handleSubmit} className="flex flex-col gap-6">
            <Card title="個人檔案可見範圍" className="flex flex-col gap-4">
                <RadioGroup.Root
                    aria-label="個人檔案可見範圍"
                    value={settings.profile_visibility}
                    onValueChange={(value) =>
                        patch({
                            profile_visibility: value as PrivacySettings["profile_visibility"]
                        })
                    }
                    className="flex flex-col gap-4"
                >
                    {visibilityOptions.map((option) => {
                        const id = `${visibilityId}-${option.value}`;
                        return (
                            <div key={option.value} className="flex items-start gap-3">
                                <RadioGroup.Item
                                    id={id}
                                    value={option.value}
                                    aria-describedby={`${id}-description`}
                                    className="mt-0.5 inline-flex size-5 shrink-0 cursor-pointer items-center justify-center rounded-full border border-ink/40 bg-surface focus-ring transition-colors hover:border-ink data-[state=checked]:border-ink"
                                >
                                    <RadioGroup.Indicator className="size-2.5 rounded-full bg-ink" />
                                </RadioGroup.Item>
                                <div className="min-w-0">
                                    <Label.Root
                                        htmlFor={id}
                                        className="cursor-pointer font-sans text-base text-ink"
                                    >
                                        {option.label}
                                    </Label.Root>
                                    <p
                                        id={`${id}-description`}
                                        className="font-sans text-sm text-copy-muted"
                                    >
                                        {option.description}
                                    </p>
                                </div>
                            </div>
                        );
                    })}
                </RadioGroup.Root>
            </Card>

            <Card
                title="個人檔案顯示內容"
                description="選擇要在個人檔案頁面上公開的項目。"
                className="flex flex-col gap-4"
            >
                <Checkbox
                    label="顯示我發佈的文章"
                    checked={settings.show_articles}
                    onCheckedChange={(show_articles) => patch({ show_articles })}
                />
                <Checkbox
                    label="顯示我的備審作品"
                    checked={settings.show_portfolio}
                    onCheckedChange={(show_portfolio) => patch({ show_portfolio })}
                />
            </Card>

            <SaveBar pending={pending !== null} saved={saved} error={error} />
        </form>
    );
}
