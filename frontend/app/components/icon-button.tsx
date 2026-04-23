import { AccessibleIcon, Slot } from "radix-ui";
import type { ComponentPropsWithoutRef, ReactNode } from "react";
import { twMerge } from "tailwind-merge";

interface IconButtonProps extends ComponentPropsWithoutRef<"button"> {
    asChild?: boolean;
    icon: ReactNode;
    label: string;
}

export default function IconButton({ asChild, className, icon, label, ...props }: IconButtonProps) {
    const Comp = asChild ? Slot.Root : "button";

    return (
        <Comp
            aria-label={label}
            className={twMerge(
                "inline-flex h-12 w-12 cursor-pointer items-center justify-center rounded-[var(--radius-control)] transition-transform duration-200 disabled:pointer-events-none disabled:opacity-50",
                className
            )}
            {...props}
        >
            <AccessibleIcon.Root label={label}>{icon}</AccessibleIcon.Root>
        </Comp>
    );
}
