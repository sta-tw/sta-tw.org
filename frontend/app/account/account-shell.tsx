"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { Loader2 } from "lucide-react";
import { useEffect } from "react";
import { twMerge } from "tailwind-merge";
import Avatar from "../components/avatar";
import { displayNameOf, profileAvatarSrc } from "../lib/api/profile";
import { useCurrentAccount } from "../lib/current-account";
import { loginHref, userHref } from "../lib/routes";
import { accountNav } from "./account-nav";

export default function AccountShell({ children }: { children: React.ReactNode }) {
    const pathname = usePathname();
    const router = useRouter();
    const { me, status, profile } = useCurrentAccount();

    useEffect(() => {
        if (status === "anonymous") router.replace(loginHref(pathname));
    }, [status, pathname, router]);

    if (!me) {
        return (
            <main className="flex flex-1 items-center justify-center bg-surface px-5 py-24">
                <Loader2 aria-label="載入中" className="size-8 animate-spin text-copy-muted" />
            </main>
        );
    }

    const name = profile ? displayNameOf(profile) : me.account.username;

    return (
        <main className="article-dots flex-1">
            <div className="mx-auto grid w-full max-w-screen-xl gap-6 px-5 py-8 sm:px-6 sm:py-10 lg:grid-cols-[14rem_minmax(0,1fr)] lg:gap-12 lg:px-16 lg:py-14">
                <aside className="min-w-0 lg:border-r lg:border-ink/10 lg:pr-8">
                    <div className="flex items-center gap-4 lg:flex-col lg:gap-3 lg:text-center">
                        <Avatar
                            name={name}
                            src={profileAvatarSrc(profile)}
                            className="lg:size-28 lg:text-4xl"
                        />
                        <div className="min-w-0">
                            <p className="truncate font-sans text-xl font-medium text-ink">
                                {name}
                            </p>
                            <Link
                                href={userHref(me.account.username)}
                                className="font-sans text-sm text-copy-muted underline-offset-4 hover:text-ink hover:underline"
                            >
                                查看個人頁面
                            </Link>
                        </div>
                    </div>
                    <nav
                        aria-label="帳號選單"
                        className="-mx-5 mt-5 overflow-x-auto px-5 sm:mx-0 sm:px-0 lg:mt-8"
                    >
                        <ul className="flex w-max gap-2 lg:w-auto lg:flex-col lg:gap-1">
                            {accountNav.map((item) => {
                                const active = pathname.startsWith(item.href);
                                return (
                                    <li key={item.href}>
                                        <Link
                                            href={item.href}
                                            aria-current={active ? "page" : undefined}
                                            className={twMerge(
                                                "block rounded-[var(--radius-small)] px-3 py-2 font-sans text-base whitespace-nowrap text-copy-muted focus-ring transition-colors hover:bg-ink/5 hover:text-ink lg:text-center",
                                                active &&
                                                    "bg-accent-green text-ink hover:bg-accent-green"
                                            )}
                                        >
                                            {item.label}
                                        </Link>
                                    </li>
                                );
                            })}
                        </ul>
                    </nav>
                </aside>
                <div className="min-w-0">{children}</div>
            </div>
        </main>
    );
}
