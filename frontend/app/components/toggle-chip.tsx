"use client";

import { Toggle, ToggleGroup } from "radix-ui";
import type { ComponentPropsWithoutRef, ReactNode } from "react";
import { twMerge } from "tailwind-merge";

const chipClass =
    "inline-flex min-h-10 cursor-pointer items-center justify-center rounded-full border border-ink/15 bg-surface px-4 py-1.5 font-sans text-sm font-medium text-copy-muted transition-colors hover:border-ink/30 hover:text-ink focus-ring disabled:cursor-not-allowed disabled:opacity-50 data-[state=on]:text-ink sm:text-base";

const toneClasses = {
    green: "data-[state=on]:border-accent-green-strong data-[state=on]:bg-accent-green",
    /** The brochure page's action pills (簡章下載, 新增至日曆) are yellow. */
    yellow: "data-[state=on]:border-transparent data-[state=on]:bg-accent-yellow/75 data-[state=on]:hover:bg-accent-yellow"
} as const;

type Tone = keyof typeof toneClasses;

/** A pressable pill in the HashtagBadge family: used for single on/off
 * choices and quick filters. For a set of choices use ToggleChipGroup. */
export default function ToggleChip({
    className,
    tone = "green",
    ...props
}: ComponentPropsWithoutRef<typeof Toggle.Root> & { tone?: Tone }) {
    return <Toggle.Root className={twMerge(chipClass, toneClasses[tone], className)} {...props} />;
}

export interface ToggleChipOption {
    value: string;
    label: ReactNode;
    disabled?: boolean;
}

/** Multi-select row of chips (學群, tags) on Radix ToggleGroup, so arrow
 * keys move between chips. Put it in a FieldGroup for the label. */
export function ToggleChipGroup({
    options,
    value,
    onValueChange,
    className
}: {
    options: readonly ToggleChipOption[];
    value: string[];
    onValueChange: (value: string[]) => void;
    className?: string;
}) {
    return (
        <ToggleGroup.Root
            type="multiple"
            value={value}
            onValueChange={onValueChange}
            className={twMerge("flex flex-wrap gap-2", className)}
        >
            {options.map((option) => (
                <ToggleGroup.Item
                    key={option.value}
                    value={option.value}
                    disabled={option.disabled}
                    className={twMerge(chipClass, toneClasses.green)}
                >
                    {option.label}
                </ToggleGroup.Item>
            ))}
        </ToggleGroup.Root>
    );
}
