import Link from "next/link";
import { ArrowRight, Home } from "lucide-react";
import { twMerge } from "tailwind-merge";
import Button from "./button";

type StatusPageAction = {
    label: string;
    href: string;
    variant?: "primary" | "secondary";
};

type StatusPageProps = {
    eyebrow: string;
    title: string;
    description: string;
    actions?: StatusPageAction[];
};

const defaultActions: StatusPageAction[] = [
    { label: "回到首頁", href: "/", variant: "primary" },
    { label: "瀏覽文章", href: "/articles", variant: "secondary" }
];

export default function StatusPage({
    eyebrow,
    title,
    description,
    actions = defaultActions
}: StatusPageProps) {
    return (
        <main className="relative isolate flex min-h-[calc(100svh-4.5rem)] flex-1 overflow-hidden bg-surface">
            <div
                aria-hidden
                className="pointer-events-none absolute -top-24 -left-28 h-[22rem] w-[22rem] rounded-full bg-accent-yellow sm:h-[30rem] sm:w-[30rem] lg:-top-32 lg:-left-36 lg:h-[42rem] lg:w-[42rem]"
            />
            <div
                aria-hidden
                className="pointer-events-none absolute right-[-10rem] bottom-[-12rem] h-[24rem] w-[24rem] rounded-full bg-accent-green sm:h-[34rem] sm:w-[34rem] lg:right-[-8rem] lg:bottom-[-18rem] lg:h-[48rem] lg:w-[48rem]"
            />
            <p
                aria-hidden
                className="pointer-events-none absolute top-24 left-5 font-serif text-watermark whitespace-nowrap text-ink/10 select-none sm:left-10 lg:left-16"
            >
                Dream it,
            </p>
            <p
                aria-hidden
                className="pointer-events-none absolute bottom-4 left-[40%] font-serif text-watermark whitespace-nowrap text-ink/10 select-none"
            >
                and make it possible.
            </p>

            <section className="relative z-10 mx-auto flex w-full max-w-screen-xl items-center px-5 py-16 sm:px-6 sm:py-20 lg:px-16 lg:py-24">
                <div className="flex max-w-3xl min-w-0 flex-col gap-8">
                    <div className="flex flex-col gap-5">
                        <p className="w-fit rounded-full bg-accent-green-strong px-4 py-2 font-sans text-sm font-bold tracking-[0.08em] text-ink uppercase sm:text-base">
                            {eyebrow}
                        </p>
                        <h1 className="font-serif text-hero-subtitle text-balance text-ink">
                            {title}
                        </h1>
                        <p className="max-w-2xl font-sans text-feature-body font-medium text-copy-muted">
                            {description}
                        </p>
                    </div>

                    <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap">
                        {actions.map((action, index) => (
                            <Button
                                key={action.href}
                                asChild
                                className={twMerge(
                                    "h-13 w-full gap-2 px-5 text-lg sm:w-fit sm:min-w-36",
                                    action.variant === "secondary" &&
                                        "border border-ink/15 bg-surface/70 text-ink hover:bg-ink/5 active:bg-ink/10"
                                )}
                            >
                                <Link href={action.href}>
                                    {index === 0 ? (
                                        <Home aria-hidden className="h-5 w-5" />
                                    ) : (
                                        <ArrowRight aria-hidden className="h-5 w-5" />
                                    )}
                                    {action.label}
                                </Link>
                            </Button>
                        ))}
                    </div>
                </div>
            </section>
        </main>
    );
}
