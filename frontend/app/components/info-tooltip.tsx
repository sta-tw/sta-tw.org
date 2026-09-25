"use client";

import { CircleHelp } from "lucide-react";
import { Tooltip } from "radix-ui";

type InfoTooltipProps = {
    children: React.ReactNode;
    label: string;
};

export default function InfoTooltip({ children, label }: InfoTooltipProps) {
    return (
        <Tooltip.Provider delayDuration={150}>
            <Tooltip.Root>
                <Tooltip.Trigger asChild>
                    <button
                        type="button"
                        aria-label={label}
                        className="inline-flex h-7 w-7 shrink-0 cursor-help items-center justify-center rounded-full text-ink/65 transition-colors hover:bg-ink/8 hover:text-ink focus-visible:ring-2 focus-visible:ring-ink focus-visible:ring-offset-2 focus-visible:outline-none"
                    >
                        <CircleHelp aria-hidden className="h-5 w-5" strokeWidth={1.9} />
                    </button>
                </Tooltip.Trigger>
                <Tooltip.Portal>
                    <Tooltip.Content
                        sideOffset={8}
                        className="z-[70] max-w-72 rounded-[var(--radius-small)] bg-ink px-3 py-2 font-sans text-sm leading-relaxed text-surface shadow-[var(--shadow-card)]"
                    >
                        {children}
                        <Tooltip.Arrow className="fill-ink" />
                    </Tooltip.Content>
                </Tooltip.Portal>
            </Tooltip.Root>
        </Tooltip.Provider>
    );
}
