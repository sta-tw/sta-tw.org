"use client";

import { Tabs as RadixTabs } from "radix-ui";
import type { ComponentPropsWithoutRef } from "react";
import { twMerge } from "tailwind-merge";

export const Tabs = RadixTabs.Root;

/** Horizontally scrollable on narrow screens so long tab sets never wrap. */
export function TabsList({ className, ...props }: ComponentPropsWithoutRef<typeof RadixTabs.List>) {
    return (
        <div className="-mx-5 overflow-x-auto px-5 pb-2 sm:mx-0 sm:px-0">
            <RadixTabs.List
                className={twMerge(
                    "flex w-max min-w-full gap-2 border-b border-ink/15 pb-3 sm:gap-3",
                    className
                )}
                {...props}
            />
        </div>
    );
}

export function TabsTrigger({
    className,
    ...props
}: ComponentPropsWithoutRef<typeof RadixTabs.Trigger>) {
    return (
        <RadixTabs.Trigger
            className={twMerge(
                "cursor-pointer rounded-[var(--radius-small)] bg-accent-green/45 px-3 py-2 font-sans text-sm font-medium whitespace-nowrap text-ink focus-ring transition-colors hover:bg-accent-green/80 data-[state=active]:bg-accent-green-strong sm:text-base",
                className
            )}
            {...props}
        />
    );
}

export function TabsContent({
    className,
    ...props
}: ComponentPropsWithoutRef<typeof RadixTabs.Content>) {
    return <RadixTabs.Content className={twMerge("outline-none", className)} {...props} />;
}
