"use client";

import { ChevronDown } from "lucide-react";
import type { ComponentPropsWithoutRef } from "react";
import { twMerge } from "tailwind-merge";
import { useFieldControl } from "./field";
import { singleLineControlClass } from "./input";

export interface SelectOption {
    value: string;
    label: string;
}

/** The label of the option with `value`, e.g. to echo a stored choice. */
export function labelOf(options: readonly SelectOption[], value?: string) {
    return options.find((option) => option.value === value)?.label;
}

interface SelectProps extends ComponentPropsWithoutRef<"select"> {
    options: readonly SelectOption[];
    /** Rendered as an empty-value first option, e.g. "請選擇". */
    placeholder?: string;
    /** Classes for the positioning wrapper (layout/width); `className` styles
     * the <select> itself, same as Input. */
    wrapperClassName?: string;
}

/** Native <select> (keeps mobile pickers and form submission working) with
 * the shared control styling and a lucide chevron. */
export default function Select({
    options,
    placeholder,
    className,
    wrapperClassName,
    ...props
}: SelectProps) {
    return (
        <div className={twMerge("relative min-w-0", wrapperClassName)}>
            <select
                className={twMerge(
                    singleLineControlClass,
                    "cursor-pointer appearance-none pr-11",
                    className
                )}
                {...useFieldControl(props)}
            >
                {placeholder !== undefined && <option value="">{placeholder}</option>}
                {options.map((option) => (
                    <option key={option.value} value={option.value}>
                        {option.label}
                    </option>
                ))}
            </select>
            <ChevronDown
                aria-hidden
                className="pointer-events-none absolute top-1/2 right-4 h-5 w-5 -translate-y-1/2 text-copy-muted"
            />
        </div>
    );
}
