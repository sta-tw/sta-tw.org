"use client";

import { CheckCircle2 } from "lucide-react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";
import Button from "../../components/button";
import { Card } from "../../components/card";
import {
    listLinkedIdentities,
    oauthErrorMessage,
    oauthProviderLabel,
    oauthProviders,
    startOAuthBind,
    type OAuthProvider
} from "../../lib/api/identities";
import { ErrorText, Resource, StatusText } from "../../components/status";
import { useAction, useResource } from "../../lib/use-resource";
import { formatDate } from "../../lib/format";

const loadIdentities = (signal: AbortSignal) =>
    listLinkedIdentities({ signal }).then(({ data }) => data);

export default function LinkedAccounts() {
    const searchParams = useSearchParams();
    const router = useRouter();
    const pathname = usePathname();
    const { state, reload } = useResource(loadIdentities);
    const { pending, error: startError, run } = useAction();

    // Read the OAuth round-trip result once, then strip it from the URL so a
    // reload doesn't show the notice again.
    const [notice] = useState((): { ok: boolean; text: string } | null => {
        const provider = searchParams.get("provider");
        const error = searchParams.get("oauth_error");
        if (error) return { ok: false, text: oauthErrorMessage(error, provider) };
        if (searchParams.get("oauth") === "success") {
            return { ok: true, text: `已成功連動 ${oauthProviderLabel(provider) ?? "帳號"}。` };
        }
        return null;
    });

    useEffect(() => {
        if (notice) router.replace(pathname, { scroll: false });
    }, [notice, router, pathname]);

    function handleLink(provider: OAuthProvider) {
        void run(provider, async () => {
            const { authorization_url } = await startOAuthBind(
                provider,
                `${pathname}?provider=${provider}`
            );
            window.location.assign(authorization_url);
            // Keep the button busy while the browser navigates away.
            await new Promise(() => undefined);
        });
    }

    return (
        <Card
            title="帳號連動"
            description="連動後可使用 Discord 或 Google 快速登入。"
            className="flex flex-col gap-4"
        >
            <StatusText tone="success">{notice?.ok && notice.text}</StatusText>
            <ErrorText>{notice && !notice.ok && notice.text}</ErrorText>
            <Resource state={state} reload={reload} loadingLabel="正在載入連動狀態">
                {(identities) => (
                    <ul className="flex flex-col divide-y divide-ink/10">
                        {oauthProviders.map((provider) => {
                            const linked = identities.find((item) => item.provider === provider.id);
                            return (
                                <li
                                    key={provider.id}
                                    className="flex flex-wrap items-center justify-between gap-3 py-4 first:pt-0 last:pb-0"
                                >
                                    <div className="min-w-0">
                                        <p className="font-sans text-base font-medium text-ink">
                                            {provider.label}
                                        </p>
                                        <p className="font-sans text-sm text-copy-muted">
                                            {linked
                                                ? `${linked.display ? `${linked.display}・` : ""}連動於 ${formatDate(linked.linked_at)}`
                                                : "尚未連動"}
                                        </p>
                                    </div>
                                    {linked ? (
                                        <span className="inline-flex items-center gap-1.5 font-sans text-sm font-medium text-ink">
                                            <CheckCircle2
                                                aria-hidden
                                                className="size-5 text-accent-green-strong"
                                            />
                                            已連動
                                        </span>
                                    ) : (
                                        <Button
                                            type="button"
                                            variant="secondary"
                                            size="base"
                                            disabled={pending !== null}
                                            onClick={() => handleLink(provider.id)}
                                        >
                                            {pending === provider.id ? "前往中…" : "前往連動"}
                                        </Button>
                                    )}
                                </li>
                            );
                        })}
                    </ul>
                )}
            </Resource>
            <ErrorText>{startError}</ErrorText>
        </Card>
    );
}
