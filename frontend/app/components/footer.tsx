import Link from "next/link";
import { DISCORD_INVITE_URL, GITHUB_REPOSITORY_URL } from "../lib/site-links";

const footerSections = [
    {
        id: "social",
        title: "社群媒體",
        links: [
            { label: "Instagram", href: "#" },
            { label: "Discord", href: DISCORD_INVITE_URL }
        ]
    },
    {
        id: "related",
        title: "相關網站",
        links: [
            { label: "綠洲計畫", href: "/links" },
            { label: "長浪計畫", href: "/links" }
        ]
    },
    {
        id: "about",
        title: "關於本站",
        links: [
            { label: "Github", href: GITHUB_REPOSITORY_URL },
            { label: "Dev. Credit", href: "/credits" }
        ]
    }
];

export default function Footer() {
    return (
        <footer className="bg-surface pt-12 pb-16 lg:pt-11 lg:pb-44">
            <div className="mx-auto grid w-full max-w-screen-xl grid-cols-1 gap-12 px-5 sm:px-6 lg:grid-cols-[minmax(0,1fr)_minmax(35rem,0.82fr)] lg:gap-16 lg:px-16">
                <Link href="/" className="block w-fit max-w-full text-ink">
                    <span className="block max-w-full font-serif text-3xl leading-tight text-ink sm:text-4xl lg:text-5xl">
                        <span>S.T.A</span>
                        <span className="mx-2 hidden sm:inline">|</span>
                        <span className="block sm:inline">特殊選才資源網</span>
                    </span>
                    <span className="mt-3 block font-serif text-base leading-relaxed text-ink/60 sm:text-lg">
                        <span>Special Talent Admission</span>
                        <span className="mx-2 hidden sm:inline">|</span>
                        <span className="block sm:inline">Information Site</span>
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
                                className="font-serif text-lg leading-snug text-ink sm:text-xl"
                            >
                                {section.title}
                            </h2>
                            <ul className="mt-4 flex flex-col gap-3 lg:mt-5">
                                {section.links.map((link) => (
                                    <li key={link.label}>
                                        <Link
                                            href={link.href}
                                            className="font-serif text-base leading-normal text-ink/60 transition-colors hover:text-ink sm:text-lg"
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
