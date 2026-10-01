import { Slot } from "radix-ui";
import type { ComponentPropsWithoutRef } from "react";
import { twMerge } from "tailwind-merge";

interface ButtonProps extends ComponentPropsWithoutRef<"button"> {
    asChild?: boolean;
    /** "primary" (default) keeps the original solid-ink pill unchanged, so
     * every existing call site is unaffected. "secondary" and "danger"
     * replace two className overrides that were being copy-pasted at 10+
     * call sites across the admin panel (see app/admin/admin-ui.tsx). */
    variant?: "primary" | "secondary" | "danger";
}

const variantClasses: Record<NonNullable<ButtonProps["variant"]>, string> = {
    primary: "",
    secondary: "border border-ink/15 bg-surface text-ink hover:bg-ink/5 active:bg-ink/10",
    danger: "bg-red-600 text-white hover:bg-red-700 active:bg-red-800"
};

export default function Button({ asChild, variant = "primary", className, children, ...props }: ButtonProps) {
    const Comp = asChild ? Slot.Root : "button";
    return (
        <Comp
            className={twMerge(
                "inline-flex h-12 cursor-pointer items-center justify-center rounded-[var(--radius-control)] bg-button px-6 font-serif text-xl font-bold text-button-foreground transition-colors hover:bg-button-hover active:bg-button-active disabled:pointer-events-none disabled:opacity-50",
                variantClasses[variant],
                className
            )}
            {...props}
        >
            {children}
        </Comp>
    );
}
