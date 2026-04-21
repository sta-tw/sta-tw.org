import Image from "next/image";
import Link from "next/link";
import { NavigationMenu } from "radix-ui";
import Button from "./button";

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
      <div className="mx-auto flex max-w-screen-xl items-center justify-between px-16 py-4">
        <div className="flex items-center gap-8">
          <Link href="/" className="flex items-center gap-3 shrink-0">
            <Image
              src={logoIcon}
              alt="S.T.A Logo"
              width={32}
              height={32}
              className="rounded-lg"
            />
            <span className="font-serif text-[28px] font-normal leading-tight tracking-[-0.64px] text-[#363535] whitespace-nowrap">
              S.T.A&nbsp;&nbsp;|&nbsp;&nbsp;特殊選才資源網
            </span>
          </Link>

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
        </div>

        <Button asChild className="w-[179px]">
          <Link href="/login">登入 | 註冊</Link>
        </Button>
      </div>
    </header>
  );
}
