import type { Metadata } from "next";
import FaqAccordion from "../components/faq-accordion";

export const metadata: Metadata = {
    title: "常見問題 | S.T.A 特殊選才資源網",
    description: "查看特殊選才競賽、獎項、備審資料與申請時程等常見問題。"
};

export default function FaqPage() {
    return (
        <main className="flex-1 bg-surface bg-[radial-gradient(circle,rgba(54,53,53,0.055)_1.5px,transparent_1.5px)] bg-[length:24px_24px]">
            <section
                aria-labelledby="faq-title"
                className="mx-auto w-full max-w-screen-xl px-5 pt-10 pb-16 sm:px-6 sm:pt-12 sm:pb-20 lg:px-16 lg:pt-12 lg:pb-24"
            >
                <div className="grid items-end gap-4 lg:grid-cols-[1fr_auto_1fr]">
                    <span aria-hidden className="hidden lg:block" />
                    <h1
                        id="faq-title"
                        className="text-center font-sans text-4xl leading-tight font-medium tracking-[-0.03em] text-ink sm:text-5xl"
                    >
                        常見問題
                    </h1>
                    <time
                        dateTime="2026-07-01"
                        className="text-center font-sans text-base text-ink sm:text-lg lg:justify-self-end lg:text-right"
                    >
                        Update Date : 2026 / 7 / 01
                    </time>
                </div>

                <div className="mt-10 sm:mt-12 lg:mt-14">
                    <FaqAccordion />
                </div>
            </section>
        </main>
    );
}
