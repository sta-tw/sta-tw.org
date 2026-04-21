import Image from "next/image";
import Link from "next/link";

const DISCORD_INVITE_URL = "https://discord.gg/3XAvXnG4rx";

export default function JoinDiscordSection() {
  return (
    <section className="bg-surface px-5 pt-2 pb-16 sm:px-6 sm:pb-20 lg:px-16 lg:pb-24" aria-labelledby="join-discord-heading">
      <div className="mx-auto max-w-screen-xl">
        <h2 id="join-discord-heading" className="sr-only">
          加入 Discord 社群
        </h2>

        <article className="overflow-hidden rounded-2xl border border-black/10 bg-white shadow-[0_6px_12px_rgba(0,0,0,0.03),0_4px_8px_rgba(0,0,0,0.02)]">
          <div className="grid grid-cols-1 md:grid-cols-[240px_minmax(0,1fr)] lg:grid-cols-[256px_minmax(0,1fr)]">
            <div aria-hidden className="relative min-h-56 overflow-hidden md:min-h-full">
              <Image
                src="/features/discord-admission.webp"
                alt=""
                fill
                sizes="(min-width: 1024px) 256px, (min-width: 768px) 240px, 100vw"
                className="object-cover object-center"
              />
            </div>

            <div className="flex min-w-0 flex-col justify-center gap-5 px-6 py-8 sm:px-8 sm:py-10 lg:px-10 lg:py-12">
              <div className="flex flex-col gap-4">
                <h3 className="text-balance font-serif text-4xl leading-tight text-ink sm:text-5xl lg:text-[4rem] lg:leading-none">
                  加入 116 特選 Discord 群
                </h3>
                <p className="max-w-3xl font-sans text-xl font-medium leading-relaxed text-copy-muted sm:text-2xl">
                  和 1200+ 位志同道合的人一起聊天、討論
                </p>
              </div>

              <Link
                href={DISCORD_INVITE_URL}
                className="w-fit font-sans text-2xl font-bold text-ink transition-opacity hover:opacity-70"
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
