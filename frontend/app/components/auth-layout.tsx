import type { ReactNode } from "react";
import Image from "next/image";
import { publicPath } from "../lib/public-path";

export default function AuthLayout({
    children,
    titleId
}: {
    children: ReactNode;
    titleId: string;
}) {
    return (
        <main className="relative isolate grid min-h-[calc(100svh+6rem)] w-full flex-1 bg-surface pb-12 font-sans lg:min-h-[calc(100svh+10rem)] lg:grid-cols-2 lg:pb-16">
            <div className="relative min-h-56 overflow-hidden bg-accent-green/30 sm:min-h-72 lg:min-h-0">
                <Image
                    src={publicPath("/login/ntu-campus.jpg")}
                    alt="國立臺灣大學的校園建築與椰子樹"
                    fill
                    sizes="(min-width: 1024px) 50vw, 100vw"
                    className="object-cover"
                    preload
                />
                <a
                    href="https://unsplash.com/photos/4ozvwEl7m6M"
                    target="_blank"
                    rel="noreferrer"
                    className="absolute right-4 bottom-4 z-10 rounded bg-black/60 px-2 py-1 text-xs text-white underline-offset-4 hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-white sm:right-6 sm:bottom-6 lg:bottom-16"
                >
                    Photo: Poh Soo Donald Soh / Unsplash
                </a>
            </div>
            <section
                aria-labelledby={titleId}
                className="article-dots flex min-w-0 items-center justify-center px-6 py-12 sm:px-12 sm:py-16 lg:px-16 lg:pt-20 lg:pb-64"
            >
                {children}
            </section>
            <div
                aria-hidden="true"
                className="pointer-events-none absolute inset-x-0 bottom-12 h-12 bg-linear-to-t from-surface to-transparent lg:bottom-16 lg:h-24"
            />
            <div
                aria-hidden="true"
                className="pointer-events-none absolute inset-x-6 bottom-0 flex items-center gap-4 sm:inset-x-12 lg:inset-x-16"
            >
                <span className="h-px flex-1 bg-ink/10" />
                <span className="size-2 rounded-full bg-accent-green-strong" />
                <span className="h-px flex-1 bg-ink/10" />
            </div>
        </main>
    );
}
