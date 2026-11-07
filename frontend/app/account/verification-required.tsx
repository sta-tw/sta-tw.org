import Link from "next/link";
import { ShieldCheck } from "lucide-react";
import Button from "../components/button";
import { Card } from "../components/card";
import { ApiError } from "../lib/api/types";

/** 403 verification_required from the portfolio / applications / results
 * endpoints (identity not yet verified as student or senior). */
export function isVerificationRequired(cause: unknown) {
    return cause instanceof ApiError && cause.code === "verification_required";
}

/** Shown instead of a feature that needs a verified student identity. */
export function VerificationRequired({ feature }: { feature: string }) {
    return (
        <Card className="flex flex-col items-start gap-4 sm:flex-row sm:items-center">
            <ShieldCheck aria-hidden className="size-10 shrink-0 text-accent-green-strong" />
            <div className="min-w-0 flex-1">
                <h2 className="font-sans text-xl font-medium text-ink">需要先完成身分驗證</h2>
                <p className="mt-1 font-sans text-base text-copy-muted">
                    {feature}僅開放已驗證的特選生使用，完成學校信箱或文件驗證後即可開始。
                </p>
            </div>
            <Button asChild size="base" className="w-full sm:w-auto">
                <Link href="/account/verification">前往驗證</Link>
            </Button>
        </Card>
    );
}
