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
    /** "md" (default) is the serif call-to-action size; "base" is the same
     * height in sans-serif for in-app forms and panels; "sm" is the compact
     * sans-serif size used for inline/row actions. */
    size?: "md" | "base" | "sm";
}

const variantClasses: Record<NonNullable<ButtonProps["variant"]>, string> = {
    primary: "",
    secondary: "border border-ink/15 bg-surface text-ink hover:bg-ink/5 active:bg-ink/10",
    danger: "bg-danger text-white hover:bg-danger/90 active:bg-danger/80"
};

const sizeClasses: Record<NonNullable<ButtonProps["size"]>, string> = {
    md: "h-12 px-6 font-serif text-xl",
    base: "h-12 px-6 font-sans text-base",
    sm: "h-10 px-4 font-sans text-sm"
};

export default function Button({
    asChild,
    variant = "primary",
    size = "md",
    className,
    children,
    ...props
}: ButtonProps) {
    const Comp = asChild ? Slot.Root : "button";
    return (
        <Comp
            className={twMerge(
                "inline-flex cursor-pointer items-center justify-center rounded-[var(--radius-control)] bg-button font-bold text-button-foreground focus-ring transition-colors hover:bg-button-hover active:bg-button-active disabled:pointer-events-none disabled:opacity-50",
                sizeClasses[size],
                variantClasses[variant],
                className
            )}
            {...props}
        >
            {children}
        </Comp>
    );
}
