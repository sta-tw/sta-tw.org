import type { ReactNode } from "react";

/** Heading + optional description shared by every /account sub-page. */
export default function AccountPage({
    title,
    description,
    actions,
    children
}: {
    title: string;
    description?: ReactNode;
    actions?: ReactNode;
    children: ReactNode;
}) {
    return (
        <section aria-labelledby="account-page-title" className="flex flex-col gap-6 sm:gap-8">
            <header className="flex flex-wrap items-end justify-between gap-4">
                <div className="min-w-0">
                    <h1
                        id="account-page-title"
                        className="font-sans text-3xl leading-tight font-medium tracking-[-0.02em] text-ink sm:text-4xl"
                    >
                        {title}
                    </h1>
                    {description && (
                        <p className="mt-2 font-sans text-base leading-relaxed text-copy-muted">
                            {description}
                        </p>
                    )}
                </div>
                {actions}
            </header>
            {children}
        </section>
    );
}
