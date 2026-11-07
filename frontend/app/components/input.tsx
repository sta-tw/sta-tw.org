"use client";

import type { ComponentPropsWithoutRef } from "react";
import { twMerge } from "tailwind-merge";
import { useFieldControl } from "./field";

/** Shared look for every form control. Uses --radius-control so controls
 * line up with Button, which they usually sit next to. */
const controlClass =
    "w-full rounded-[var(--radius-control)] border border-ink/20 bg-surface px-4 font-sans text-base text-ink transition-colors placeholder:text-ink/45 hover:border-ink/35 focus:border-ink focus:ring-2 focus:ring-ink/15 focus:outline-none disabled:cursor-not-allowed disabled:opacity-60 aria-invalid:border-danger read-only:bg-ink/5";

/** Height for single-line controls (Input, Select), matching Button's h-12. */
export const singleLineControlClass = twMerge(controlClass, "h-12");

export function Input({ className, ...props }: ComponentPropsWithoutRef<"input">) {
    return (
        <input className={twMerge(singleLineControlClass, className)} {...useFieldControl(props)} />
    );
}

export function Textarea({ className, ...props }: ComponentPropsWithoutRef<"textarea">) {
    return (
        <textarea
            className={twMerge(controlClass, "min-h-32 resize-y py-3 leading-relaxed", className)}
            {...useFieldControl(props)}
        />
    );
}
