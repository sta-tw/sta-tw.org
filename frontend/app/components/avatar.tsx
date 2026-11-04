"use client";

import { Avatar as RadixAvatar } from "radix-ui";
import { twMerge } from "tailwind-merge";

const sizeClasses = {
    sm: "size-10 text-base",
    md: "size-16 text-2xl",
    lg: "size-28 text-4xl",
    xl: "size-36 text-5xl"
} as const;

interface AvatarProps {
    name: string;
    src?: string | null;
    size?: keyof typeof sizeClasses;
    className?: string;
}

/** Round avatar (sizes are fixed; callers add responsive overrides) with an initial fallback while loading or without a photo. */
export default function Avatar({ name, src, size = "md", className }: AvatarProps) {
    return (
        <RadixAvatar.Root
            className={twMerge(
                "inline-flex shrink-0 items-center justify-center overflow-hidden rounded-full bg-accent-green align-middle select-none",
                sizeClasses[size],
                className
            )}
        >
            {src && <RadixAvatar.Image src={src} alt={name} className="size-full object-cover" />}
            <RadixAvatar.Fallback
                delayMs={src ? 300 : 0}
                className="font-serif text-ink"
                aria-label={name}
            >
                {Array.from(name.trim())[0]?.toUpperCase() ?? "?"}
            </RadixAvatar.Fallback>
        </RadixAvatar.Root>
    );
}
