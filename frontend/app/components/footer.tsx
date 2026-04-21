import Link from "next/link";
import { DISCORD_INVITE_URL, GITHUB_REPOSITORY_URL } from "../lib/site-links";

const footerSections = [
  {
    id: "social",
    title: "社群媒體",
    links: [
      { label: "Instagram", href: "#" },
      { label: "Discord", href: DISCORD_INVITE_URL },
    ],
  },
  {
    id: "related",
    title: "相關網站",
    links: [
      { label: "綠洲計畫", href: "/links" },
      { label: "長浪計畫", href: "/links" },
    ],
  },
  {
    id: "about",
    title: "關於本站",
    links: [
      { label: "Github", href: GITHUB_REPOSITORY_URL },
      { label: "Dev. Credit", href: "/credits" },
    ],
  },
];

export default function Footer() {
  return (
    <footer className="bg-surface pt-12 pb-16 lg:pt-11 lg:pb-44">
      <div className="mx-auto grid w-full max-w-screen-xl grid-cols-1 gap-12 px-5 sm:px-6 lg:grid-cols-[minmax(0,1fr)_minmax(35rem,0.82fr)] lg:gap-16 lg:px-16">
        <Link href="/" className="block w-fit max-w-full text-ink">
          <span className="block max-w-full font-serif text-4xl leading-tight text-ink sm:text-5xl lg:text-6xl">
            S.T.A | 特殊選才資源網
          </span>
          <span className="mt-4 block font-serif text-xl leading-relaxed text-ink/60 sm:text-2xl lg:mt-3">
            Special Talent Admission | Information Site
          </span>
        </Link>

        <nav
          aria-label="頁尾導覽"
          className="grid grid-cols-1 gap-9 sm:grid-cols-3 sm:gap-8 lg:gap-16"
        >
          {footerSections.map((section) => (
            <section key={section.id} aria-labelledby={`footer-${section.id}`}>
              <h2
                id={`footer-${section.id}`}
                className="font-serif text-2xl leading-normal text-ink sm:text-3xl"
              >
                {section.title}
              </h2>
              <ul className="mt-8 flex flex-col gap-5 lg:mt-10">
                {section.links.map((link) => (
                  <li key={link.label}>
                    <Link
                      href={link.href}
                      className="font-serif text-xl leading-normal text-ink/60 transition-colors hover:text-ink sm:text-2xl"
                    >
                      {link.label}
                    </Link>
                  </li>
                ))}
              </ul>
            </section>
          ))}
        </nav>
      </div>
    </footer>
  );
}
