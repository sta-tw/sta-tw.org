"use client";

import { AlertDialog } from "radix-ui";
import type { ReactNode } from "react";
import Button from "./button";

interface ConfirmDialogProps {
    /** The element that opens the dialog, e.g. a Button. */
    trigger: ReactNode;
    title: ReactNode;
    description: ReactNode;
    confirmLabel: string;
    /** "danger" for destructive actions, "primary" for irreversible but
     * expected ones (e.g. submitting locked data). */
    tone?: "danger" | "primary";
    onConfirm: () => void;
}

/** Radix AlertDialog for confirming a destructive or irreversible action.
 * The dialog closes on confirm; the caller owns pending/error state. */
export default function ConfirmDialog({
    trigger,
    title,
    description,
    confirmLabel,
    tone = "danger",
    onConfirm
}: ConfirmDialogProps) {
    return (
        <AlertDialog.Root>
            <AlertDialog.Trigger asChild>{trigger}</AlertDialog.Trigger>
            <AlertDialog.Portal>
                <AlertDialog.Overlay className="fixed inset-0 z-[80] bg-ink/40" />
                <AlertDialog.Content className="fixed top-1/2 left-1/2 z-[90] flex max-h-[calc(100svh-2.5rem)] w-[calc(100vw-2.5rem)] max-w-md -translate-x-1/2 -translate-y-1/2 flex-col gap-3 overflow-y-auto rounded-[var(--radius-panel)] bg-surface p-6 shadow-[var(--shadow-card)] sm:p-7">
                    <AlertDialog.Title className="font-sans text-xl font-medium text-ink">
                        {title}
                    </AlertDialog.Title>
                    <AlertDialog.Description className="font-sans text-base leading-relaxed text-copy-muted">
                        {description}
                    </AlertDialog.Description>
                    <div className="mt-3 flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
                        <AlertDialog.Cancel asChild>
                            <Button variant="secondary" size="base">
                                取消
                            </Button>
                        </AlertDialog.Cancel>
                        <AlertDialog.Action asChild>
                            <Button variant={tone} size="base" onClick={onConfirm}>
                                {confirmLabel}
                            </Button>
                        </AlertDialog.Action>
                    </div>
                </AlertDialog.Content>
            </AlertDialog.Portal>
        </AlertDialog.Root>
    );
}
