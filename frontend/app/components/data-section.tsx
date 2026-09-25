"use client";

import { useEffect, useState } from "react";
import { getDiscordCommunityStats } from "../lib/api/community";
import { getPublicStats } from "../lib/api/public-stats";

type Stat = {
    number: string;
    label: string;
};

const FALLBACK_STATS: Stat[] = [
    { number: "-", label: "特選簡章數" },
    { number: "-", label: "網站已註冊人數" },
    { number: "-", label: "DC 群人數" }
];

export default function DataSection() {
    const [stats, setStats] = useState<Stat[]>(FALLBACK_STATS);

    useEffect(() => {
        const controller = new AbortController();

        Promise.allSettled([
            getPublicStats(controller.signal),
            getDiscordCommunityStats(controller.signal)
        ]).then(([publicStatsResult, discordResult]) => {
            if (controller.signal.aborted) return;
            setStats((previous) => {
                const next = [...previous];
                if (publicStatsResult.status === "fulfilled") {
                    next[0] = {
                        ...next[0],
                        number: publicStatsResult.value.data.brochure_count.toLocaleString("zh-TW")
                    };
                    next[1] = {
                        ...next[1],
                        number: publicStatsResult.value.data.registered_accounts.toLocaleString("zh-TW")
                    };
                }
                if (discordResult.status === "fulfilled") {
                    next[2] = {
                        ...next[2],
                        number: discordResult.value.member_count.toLocaleString("zh-TW") + "+"
                    };
                }
                return next;
            });
        });

        return () => controller.abort();
    }, []);

    return (
        <section className="relative min-h-[530px] overflow-hidden bg-surface">
            <p
                aria-hidden
                className="pointer-events-none absolute top-5 left-0 font-serif text-watermark whitespace-nowrap text-ink/20 select-none"
            >
                Dream it,
            </p>
            <p
                aria-hidden
                className="pointer-events-none absolute bottom-4 left-[40%] font-serif text-watermark whitespace-nowrap text-ink/20 select-none"
            >
                and make it possible.
            </p>

            <div className="relative mx-auto flex max-w-[1275px] flex-col items-center justify-around gap-8 px-5 pt-[105px] pb-16 sm:px-6 lg:flex-row lg:px-16">
                {stats.map((stat, i) => (
                    <div
                        key={i}
                        className="relative flex h-64 w-64 items-center justify-center rounded-full bg-accent-green-strong sm:h-72 sm:w-72"
                    >
                        <div className="absolute h-56 w-56 rounded-full bg-accent-green sm:h-64 sm:w-64" />
                        <div className="relative flex flex-col items-center">
                            <span className="font-serif text-stat-number text-ink">
                                {stat.number}
                            </span>
                            <span className="font-serif text-3xl leading-[1.45] text-ink">
                                {stat.label}
                            </span>
                        </div>
                    </div>
                ))}
            </div>
        </section>
    );
}
