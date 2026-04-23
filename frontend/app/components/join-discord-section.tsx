import Image from "next/image";
import Link from "next/link";
import { DISCORD_INVITE_URL } from "../lib/site-links";

export default function JoinDiscordSection() {
  return (
    <section className="bg-surface pt-2 pb-12 sm:pb-16 lg:pb-20" aria-labelledby="join-discord-heading">
      <div className="mx-auto max-w-screen-xl px-5 sm:px-6 lg:px-16">
        <h2 id="join-discord-heading" className="sr-only">
          加入 Discord 社群
        </h2>

        <article className="overflow-hidden rounded-[var(--radius-panel)] border border-black/10 bg-white shadow-[var(--shadow-card)]">
          <div className="grid grid-cols-1 md:grid-cols-[200px_minmax(0,1fr)] lg:grid-cols-[224px_minmax(0,1fr)]">
            <div aria-hidden className="relative min-h-44 overflow-hidden md:min-h-full">
              <Image
                src="/features/discord-admission.webp"
                alt=""
                fill
                sizes="(min-width: 1024px) 224px, (min-width: 768px) 200px, 100vw"
                className="object-cover object-center"
              />
            </div>

            <div className="flex min-w-0 flex-col justify-center gap-4 px-5 py-6 sm:px-7 sm:py-8 lg:px-8 lg:py-9">
              <div className="flex flex-col gap-3">
                <h3 className="text-balance font-serif text-3xl leading-tight text-ink sm:text-4xl lg:text-[2.75rem]">
                  加入 116 特選 Discord 群
                </h3>
                <p className="max-w-3xl font-sans text-base font-medium leading-relaxed text-copy-muted sm:text-lg">
                  和 1200+ 位志同道合的人一起聊天、討論
                </p>
              </div>

              <Link
                href={DISCORD_INVITE_URL}
                className="w-fit font-sans text-lg font-bold text-ink transition-opacity hover:opacity-70 sm:text-xl"
              >
                點我加入 &rarr;
              </Link>
            </div>
          </div>
        </article>
      </div>
    </section>
  );
}
