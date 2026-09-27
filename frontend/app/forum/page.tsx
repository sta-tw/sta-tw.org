import type { Metadata } from "next";

export const metadata: Metadata = {
    title: "論壇施工中 | S.T.A 特殊選才資源網",
    description: "論壇功能目前施工中，暫未開放。",
    robots: {
        index: false,
        follow: false
    }
};

export default function ForumPage() {
    return (
        <main className="mx-auto flex min-h-[50vh] w-full max-w-screen-xl items-center justify-center px-5 py-12 sm:px-6 sm:py-16 lg:px-16 lg:py-20">
            <section
                className="w-full max-w-2xl rounded-[var(--radius-panel)] bg-accent-green/45 px-6 py-16 text-center sm:px-10 sm:py-20"
                aria-labelledby="forum-coming-soon-title"
            >
                <h1 id="forum-coming-soon-title" className="font-serif text-4xl text-ink sm:text-5xl">
                    論壇施工中
                </h1>
                <p className="mt-5 font-sans text-lg text-ink/70 sm:text-xl">
                    論壇功能目前施工中，暫未開放，敬請期待。
                </p>
            </section>
        </main>
    );
}
