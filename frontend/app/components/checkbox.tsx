"use client";

import { Check } from "lucide-react";
import { Checkbox as RadixCheckbox, Label } from "radix-ui";
import { useId, type ReactNode } from "react";
import { twMerge } from "tailwind-merge";

interface CheckboxProps {
    /** Visible label. Omit only when the box is labelled elsewhere, via
     * `aria-label` or an external <label htmlFor={id}>. */
    label?: ReactNode;
    id?: string;
    "aria-label"?: string;
    checked: boolean;
    onCheckedChange: (checked: boolean) => void;
    name?: string;
    disabled?: boolean;
    className?: string;
}

export default function Checkbox({
    label,
    checked,
    onCheckedChange,
    name,
    disabled,
    className,
    id: idProp,
    "aria-label": ariaLabel
}: CheckboxProps) {
    const generatedId = useId();
    const id = idProp ?? generatedId;
    return (
        <div className={twMerge("flex items-center gap-3", className)}>
            <RadixCheckbox.Root
                id={id}
                aria-label={ariaLabel}
                name={name}
                checked={checked}
                disabled={disabled}
                onCheckedChange={(value) => onCheckedChange(value === true)}
                className="inline-flex size-5 shrink-0 cursor-pointer items-center justify-center rounded-[calc(var(--radius-small)/2)] border border-ink/40 bg-surface text-surface focus-ring transition-colors hover:border-ink disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:border-ink data-[state=checked]:bg-ink"
            >
                <RadixCheckbox.Indicator>
                    <Check aria-hidden className="size-3.5" strokeWidth={3} />
                </RadixCheckbox.Indicator>
            </RadixCheckbox.Root>
            {label !== undefined && (
                <Label.Root
                    htmlFor={id}
                    className="cursor-pointer font-sans text-sm text-ink sm:text-base"
                >
                    {label}
                </Label.Root>
            )}
        </div>
    );
}
