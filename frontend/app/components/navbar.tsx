"use client";

import Image from "next/image";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ChevronDown, LogOut } from "lucide-react";
import { useState } from "react";
import { Accordion, Collapsible, DropdownMenu, HoverCard } from "radix-ui";
import { logout } from "../lib/api/auth";
import { displayNameOf, profileAvatarSrc } from "../lib/api/profile";
import { notifyAuthChanged } from "../lib/auth-events";
import { useCurrentAccount, type CurrentAccountState } from "../lib/current-account";
import { publicPath } from "../lib/public-path";
import { relatedSites } from "../lib/related-sites";
import { userHref } from "../lib/routes";
import Avatar from "./avatar";
import Button from "./button";
import styles from "./navbar.module.css";

const logoIcon = publicPath("/logo.svg");

const navLinks = [
    { label: "文章總覽", href: "/articlemain" },
    { label: "簡章搜尋", href: "/bochures" },
    { label: "討論區", href: "/forum" }
];

type NavLink = { label: string; href: string };

/** The signed-in user's menu entries; admin links keep the old rule (full
 * admins get 後台管理, admissions moderators only 簡章管理). */
function accountLinks(me: NonNullable<CurrentAccountState["me"]>): NavLink[] {
    const links = [
        { label: "我的帳號", href: "/account/profile" },
        { label: "個人頁面", href: userHref(me.account.username) }
    ];
    if (me.is_admin) links.push({ label: "後台管理", href: "/admin" });
    else if (me.can_manage_admissions) links.push({ label: "簡章管理", href: "/admin/admissions" });
    return links;
}

/** Ends the session, tells the rest of the page, and goes home. Clearing the
 * local state even when the request fails keeps the UI from looking stuck. */
function useSignOut() {
    const router = useRouter();
    return () => {
        void logout()
            .catch(() => undefined)
            .finally(() => {
                notifyAuthChanged();
                router.push("/");
            });
    };
}

export default function Navbar() {
    const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);
    const closeMobileMenu = () => setIsMobileMenuOpen(false);
    const account = useCurrentAccount();

    return (
        <header className="sticky top-0 z-50 w-full border-b border-ink/10 bg-surface/95 backdrop-blur-sm">
            <Collapsible.Root
                className="mx-auto max-w-screen-xl px-4 sm:px-6 lg:px-16"
                open={isMobileMenuOpen}
                onOpenChange={setIsMobileMenuOpen}
            >
                <div className="flex items-center justify-between gap-4 py-3 sm:py-4">
                    <Link
                        href="/"
                        onClick={closeMobileMenu}
                        className="flex min-w-0 flex-1 items-center gap-3 pr-2 lg:flex-none lg:pr-0"
                    >
                        <Image
                            src={logoIcon}
                            alt="S.T.A Logo"
                            width={32}
                            height={32}
                            className="shrink-0"
                        />
                        <span className="min-w-0 font-serif text-lg leading-tight font-normal tracking-[-0.02em] text-ink sm:text-2xl lg:text-brand lg:whitespace-nowrap">
                            <span>S.T.A</span>
                            <span className="mx-2 hidden sm:inline">|</span>
                            <span className="block sm:inline">特殊選才資源網</span>
                        </span>
                    </Link>

                    <DesktopNavigation account={account} />

                    <Collapsible.Trigger
                        className={`${styles.menuTrigger} inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-[var(--radius-control)] border border-ink/15 text-ink transition-colors hover:bg-ink/5 lg:hidden`}
                        aria-label="切換導覽選單"
                    >
                        <span className="sr-only">切換導覽選單</span>
                        <span className="relative h-4 w-5">
                            <span
                                className={`${styles.menuTriggerBar} absolute top-0 left-0 block h-0.5 w-5 rounded-full bg-current`}
                            />
                            <span
                                className={`${styles.menuTriggerBar} absolute top-[7px] left-0 block h-0.5 w-5 rounded-full bg-current`}
                            />
                            <span
                                className={`${styles.menuTriggerBar} absolute top-[14px] left-0 block h-0.5 w-5 rounded-full bg-current`}
                            />
                        </span>
                    </Collapsible.Trigger>
                </div>

                <MobileNavigation onNavigate={closeMobileMenu} account={account} />
            </Collapsible.Root>
        </header>
    );
}

function DesktopNavigation({ account }: { account: CurrentAccountState }) {
    return (
        <nav aria-label="Primary navigation" className="hidden items-center gap-8 lg:flex">
            <div className="flex items-center gap-8">
                {navLinks.map((link) => (
                    <Link
                        key={link.href}
                        href={link.href}
                        className="font-sans text-nav tracking-[-0.1px] text-ink/75 transition-colors hover:text-ink"
                    >
                        {link.label}
                    </Link>
                ))}
                <Link
                    href="/report-data"
                    className="hidden font-sans text-nav tracking-[-0.1px] text-ink/75 transition-colors hover:text-ink xl:inline-flex"
                >
                    資料回報
                </Link>
                <RelatedSitesDesktopMenu />
            </div>

            {account.status === "authenticated" ? (
                <AccountMenu account={account} />
            ) : account.status === "anonymous" ? (
                <Button asChild>
                    <Link href="/login">登入 | 註冊</Link>
                </Button>
            ) : (
                // Reserve the avatar's footprint while /auth/me is loading.
                <span aria-hidden className="size-10" />
            )}
        </nav>
    );
}

type SignedIn = Extract<CurrentAccountState, { status: "authenticated" }>;

function AccountAvatar({ account }: { account: SignedIn }) {
    const name = account.profile ? displayNameOf(account.profile) : account.me.account.username;
    return <Avatar name={name} src={profileAvatarSrc(account.profile)} size="sm" />;
}

const menuItemClass =
    "flex cursor-pointer items-center gap-2 rounded-[calc(var(--radius-small)-0.25rem)] px-3 py-2 font-sans text-base whitespace-nowrap text-ink/75 outline-none transition-colors data-[highlighted]:bg-ink/5 data-[highlighted]:text-ink";

function AccountMenu({ account }: { account: SignedIn }) {
    const signOut = useSignOut();
    const { username } = account.me.account;

    return (
        <DropdownMenu.Root modal={false}>
            <DropdownMenu.Trigger
                aria-label={`${username} 的帳號選單`}
                className="cursor-pointer rounded-full focus-ring"
            >
                <AccountAvatar account={account} />
            </DropdownMenu.Trigger>
            <DropdownMenu.Portal>
                <DropdownMenu.Content
                    align="end"
                    sideOffset={12}
                    className={`${styles.accountMenu} z-[70] min-w-44 rounded-[var(--radius-small)] border border-ink/10 bg-surface p-2 shadow-[var(--shadow-card)]`}
                >
                    <DropdownMenu.Label className="truncate px-3 pt-1 pb-2 font-sans text-sm text-copy-muted">
                        {username}
                    </DropdownMenu.Label>
                    {accountLinks(account.me).map((link) => (
                        <DropdownMenu.Item key={link.href} asChild className={menuItemClass}>
                            <Link href={link.href}>{link.label}</Link>
                        </DropdownMenu.Item>
                    ))}
                    <DropdownMenu.Separator className="my-1 h-px bg-ink/10" />
                    <DropdownMenu.Item onSelect={signOut} className={menuItemClass}>
                        <LogOut aria-hidden className="size-4" />
                        登出
                    </DropdownMenu.Item>
                </DropdownMenu.Content>
            </DropdownMenu.Portal>
        </DropdownMenu.Root>
    );
}

function RelatedSitesDesktopMenu() {
    return (
        <HoverCard.Root openDelay={80} closeDelay={120}>
            <HoverCard.Trigger asChild>
                <button
                    type="button"
                    className="cursor-pointer appearance-none border-0 bg-transparent p-0 font-sans text-nav tracking-[-0.1px] text-ink/75 transition-colors hover:text-ink data-[state=open]:text-ink"
                >
                    相關網站
                </button>
            </HoverCard.Trigger>
            <HoverCard.Portal>
                <HoverCard.Content
                    align="end"
                    sideOffset={16}
                    className={`${styles.relatedSitesPanel} z-[70] min-w-40 rounded-[var(--radius-small)] border border-ink/10 bg-surface p-2 shadow-[var(--shadow-card)]`}
                >
                    <HoverCard.Arrow className="fill-surface" />
                    {relatedSites.map((site) => (
                        <Link
                            key={site.href}
                            href={site.href}
                            className="block rounded-[calc(var(--radius-small)-0.25rem)] px-3 py-2 font-sans text-base whitespace-nowrap text-ink/75 transition-colors hover:bg-ink/5 hover:text-ink"
                        >
                            {site.label}
                        </Link>
                    ))}
                </HoverCard.Content>
            </HoverCard.Portal>
        </HoverCard.Root>
    );
}

const mobileLinkClass =
    "rounded-[var(--radius-control)] px-3 py-3 font-sans text-lg tracking-[-0.1px] text-ink transition-colors hover:bg-ink/5";

function MobileNavigation({
    onNavigate,
    account
}: {
    onNavigate: () => void;
    account: CurrentAccountState;
}) {
    const signOut = useSignOut();

    return (
        <Collapsible.Content
            className={`${styles.menuContent} border-t border-ink/10 pb-4 lg:hidden`}
        >
            <nav aria-label="Mobile navigation" className="flex flex-col gap-1 pt-4">
                {navLinks.map((link) => (
                    <Link
                        key={link.href}
                        href={link.href}
                        onClick={onNavigate}
                        className={mobileLinkClass}
                    >
                        {link.label}
                    </Link>
                ))}
                <Link href="/report-data" onClick={onNavigate} className={mobileLinkClass}>
                    資料回報
                </Link>
                <RelatedSitesMobileMenu onNavigate={onNavigate} />
            </nav>

            {account.status === "authenticated" ? (
                <section
                    aria-label="帳號"
                    className="mt-4 flex flex-col gap-1 border-t border-ink/10 pt-4"
                >
                    <div className="flex items-center gap-3 px-3 pb-2">
                        <AccountAvatar account={account} />
                        <span className="min-w-0 truncate font-sans text-lg text-ink">
                            {account.me.account.username}
                        </span>
                    </div>
                    {accountLinks(account.me).map((link) => (
                        <Link
                            key={link.href}
                            href={link.href}
                            onClick={onNavigate}
                            className={mobileLinkClass}
                        >
                            {link.label}
                        </Link>
                    ))}
                    <button
                        type="button"
                        onClick={() => {
                            onNavigate();
                            signOut();
                        }}
                        className={`${mobileLinkClass} flex cursor-pointer items-center gap-2 text-left`}
                    >
                        <LogOut aria-hidden className="size-5" />
                        登出
                    </button>
                </section>
            ) : account.status === "anonymous" ? (
                <Button asChild className="mt-4 w-full">
                    <Link href="/login" onClick={onNavigate}>
                        登入 | 註冊
                    </Link>
                </Button>
            ) : null}
        </Collapsible.Content>
    );
}

function RelatedSitesMobileMenu({ onNavigate }: { onNavigate: () => void }) {
    return (
        <Accordion.Root type="single" collapsible>
            <Accordion.Item value="related-sites">
                <Accordion.Header>
                    <Accordion.Trigger className="group flex w-full items-center justify-between rounded-[var(--radius-control)] px-3 py-3 font-sans text-lg tracking-[-0.1px] text-ink transition-colors hover:bg-ink/5">
                        相關網站
                        <ChevronDown
                            aria-hidden
                            className="h-4 w-4 transition-transform duration-200 group-data-[state=open]:rotate-180"
                        />
                    </Accordion.Trigger>
                </Accordion.Header>
                <Accordion.Content className={styles.mobileRelatedSitesContent}>
                    <div className="flex flex-col gap-1 pl-3">
                        {relatedSites.map((site) => (
                            <Link
                                key={site.href}
                                href={site.href}
                                onClick={onNavigate}
                                className="rounded-[var(--radius-control)] px-3 py-2.5 font-sans text-base tracking-[-0.1px] text-ink/75 transition-colors hover:bg-ink/5 hover:text-ink"
                            >
                                {site.label}
                            </Link>
                        ))}
                    </div>
                </Accordion.Content>
            </Accordion.Item>
        </Accordion.Root>
    );
}
