"use client";

import { Label, RadioGroup as RadixRadioGroup } from "radix-ui";
import { useId, type ReactNode } from "react";
import { twMerge } from "tailwind-merge";

export interface RadioOption {
    value: string;
    label: ReactNode;
    /** Muted line under the label. */
    description?: ReactNode;
    disabled?: boolean;
}

/** Single choice from a short list, on Radix RadioGroup (arrow keys move
 * between options). Put it in a FieldGroup for the label. */
export default function RadioGroup({
    options,
    value,
    onValueChange,
    name,
    className
}: {
    options: readonly RadioOption[];
    value: string;
    onValueChange: (value: string) => void;
    name?: string;
    className?: string;
}) {
    const id = useId();
    return (
        <RadixRadioGroup.Root
            value={value}
            onValueChange={onValueChange}
            name={name}
            className={twMerge("flex flex-col gap-3", className)}
        >
            {options.map((option) => {
                const optionId = `${id}-${option.value}`;
                return (
                    <div key={option.value} className="flex items-start gap-3">
                        <RadixRadioGroup.Item
                            id={optionId}
                            value={option.value}
                            disabled={option.disabled}
                            className="mt-0.5 inline-flex size-5 shrink-0 cursor-pointer items-center justify-center rounded-full border border-ink/40 bg-surface focus-ring transition-colors hover:border-ink disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:border-ink"
                        >
                            <RadixRadioGroup.Indicator className="size-2.5 rounded-full bg-ink" />
                        </RadixRadioGroup.Item>
                        <div className="min-w-0">
                            <Label.Root
                                htmlFor={optionId}
                                className={twMerge(
                                    "cursor-pointer font-sans text-base text-ink",
                                    option.disabled && "cursor-not-allowed text-copy-muted"
                                )}
                            >
                                {option.label}
                            </Label.Root>
                            {option.description && (
                                <p className="font-sans text-sm text-copy-muted">
                                    {option.description}
                                </p>
                            )}
                        </div>
                    </div>
                );
            })}
        </RadixRadioGroup.Root>
    );
}
