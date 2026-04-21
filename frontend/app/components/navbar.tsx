import Image from "next/image";
import Link from "next/link";
import { Collapsible, NavigationMenu } from "radix-ui";
import Button from "./button";
import styles from "./navbar.module.css";

const logoIcon = "/logo.png";

const navLinks = [
  { label: "文章總覽", href: "/articles" },
  { label: "簡章搜尋", href: "/search" },
  { label: "論壇", href: "/forum" },
  { label: "相關網站", href: "/links" },
];

export default function Navbar() {
  return (
    <header className="sticky top-0 z-50 w-full border-b border-[rgba(54,53,53,0.1)] bg-white/95 backdrop-blur-sm">
      <Collapsible.Root className="mx-auto max-w-screen-xl px-4 sm:px-6 lg:px-16">
        <div className="flex items-center justify-between gap-4 py-3 sm:py-4">
          <Link href="/" className="flex min-w-0 flex-1 items-center gap-3 pr-2 lg:flex-none lg:pr-0">
            <Image
              src={logoIcon}
              alt="S.T.A Logo"
              width={32}
              height={32}
              className="shrink-0 rounded-lg"
            />
            <span className="min-w-0 font-serif text-lg font-normal leading-tight tracking-[-0.02em] text-[#363535] sm:text-2xl lg:text-[28px] lg:whitespace-nowrap">
              <span>S.T.A</span>
              <span className="mx-2 hidden sm:inline">|</span>
              <span className="block sm:inline">特殊選才資源網</span>
            </span>
          </Link>

          <div className="hidden items-center gap-8 lg:flex">
            <NavigationMenu.Root>
              <NavigationMenu.List className="flex items-center gap-8">
                {navLinks.map((link) => (
                  <NavigationMenu.Item key={link.href}>
                    <NavigationMenu.Link asChild>
                      <Link
                        href={link.href}
                        className="font-sans text-[20px] tracking-[-0.1px] text-[rgba(54,53,53,0.75)] transition-colors hover:text-[#363535]"
                      >
                        {link.label}
                      </Link>
                    </NavigationMenu.Link>
                  </NavigationMenu.Item>
                ))}
              </NavigationMenu.List>
            </NavigationMenu.Root>

            <Button asChild>
              <Link href="/login">登入 | 註冊</Link>
            </Button>
          </div>

          <Collapsible.Trigger
            className={`${styles.menuTrigger} inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-[rgba(54,53,53,0.14)] text-[#363535] transition-colors hover:bg-[rgba(54,53,53,0.04)] lg:hidden`}
            aria-label="切換導覽選單"
          >
            <span className="sr-only">切換導覽選單</span>
            <span className="relative h-4 w-5">
              <span className={`${styles.menuTriggerBar} absolute top-0 left-0 block h-0.5 w-5 rounded-full bg-current`} />
              <span className={`${styles.menuTriggerBar} absolute top-[7px] left-0 block h-0.5 w-5 rounded-full bg-current`} />
              <span className={`${styles.menuTriggerBar} absolute top-[14px] left-0 block h-0.5 w-5 rounded-full bg-current`} />
            </span>
          </Collapsible.Trigger>
        </div>

        <Collapsible.Content className={`${styles.menuContent} border-t border-[rgba(54,53,53,0.08)] pb-4 lg:hidden`}>
          <nav aria-label="Mobile navigation" className="flex flex-col gap-1 pt-4">
            {navLinks.map((link) => (
              <Link
                key={link.href}
                href={link.href}
                className="rounded-xl px-3 py-3 font-sans text-lg tracking-[-0.1px] text-[#363535] transition-colors hover:bg-[rgba(54,53,53,0.04)]"
              >
                {link.label}
              </Link>
            ))}
          </nav>

          <Button asChild className="mt-4 w-full">
            <Link href="/login">登入 | 註冊</Link>
          </Button>
        </Collapsible.Content>
      </Collapsible.Root>
    </header>
  );
}
