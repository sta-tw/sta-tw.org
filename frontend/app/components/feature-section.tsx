import Image from "next/image";
import Link from "next/link";
import { twMerge } from "tailwind-merge";
import Button from "./button";

type Feature = {
    title: string;
    body: string;
    href: string;
    image: string;
    imageAlt: string;
    imagePosition: "left" | "right";
};

const features: Feature[] = [
    {
        title: "點亮特選之路",
        body: "從備審方向、校系資料到申請時程，整理特殊選才需要掌握的資訊，讓準備過程更有脈絡，也更容易找到下一步。",
        href: "/articles",
        image: "/features/feature-road.png",
        imageAlt: "由點狀路徑組成的抽象探索圖像",
        imagePosition: "right"
    },
    {
        title: "只要有才華，\n不必擔心資訊差",
        body: "特殊選才資源網致力於透過線上資源匯集，減少特選資源分布不均等情況，並輔以論壇功能，讓大家有問題都能即時發問。",
        href: "/forum",
        image: "/features/feature-community.png",
        imageAlt: "柔和色塊交疊的抽象社群圖像",
        imagePosition: "left"
    }
];

export default function FeatureSection() {
    return (
        <section className="bg-surface" aria-labelledby="features-heading">
            <h2 id="features-heading" className="sr-only">
                特殊選才資源網特色
            </h2>
            <div className="mx-auto flex max-w-screen-xl flex-col">
                {features.map((feature, index) => (
                    <FeatureRow key={feature.title} feature={feature} isFirst={index === 0} />
                ))}
            </div>
        </section>
    );
}

function FeatureRow({ feature, isFirst }: { feature: Feature; isFirst: boolean }) {
    const imageFirst = feature.imagePosition === "left";

    return (
        <article
            className={twMerge(
                "grid grid-cols-1 items-center gap-8 px-5 py-12 sm:gap-10 sm:px-6 sm:py-16 lg:grid-cols-2 lg:gap-16 lg:px-16",
                isFirst ? "lg:pt-[120px] lg:pb-10" : "lg:pt-10 lg:pb-[120px]"
            )}
        >
            <FeatureCopy feature={feature} className={imageFirst ? "lg:order-2" : undefined} />
            <FeatureImage feature={feature} className={imageFirst ? "lg:order-1" : undefined} />
        </article>
    );
}

function FeatureCopy({ feature, className }: { feature: Feature; className?: string }) {
    return (
        <div
            className={twMerge(
                "flex min-w-0 flex-col justify-center gap-8 sm:gap-10 lg:min-h-[432px] lg:gap-12",
                className
            )}
        >
            <div className="flex flex-col gap-5 sm:gap-6">
                <h3 className="max-w-2xl font-serif text-feature-title text-balance whitespace-pre-line text-ink">
                    {feature.title}
                </h3>
                <p className="max-w-2xl font-sans text-feature-body font-medium text-copy-muted">
                    {feature.body}
                </p>
            </div>

            <Button asChild className="h-14 w-fit min-w-40 px-4 text-2xl leading-snug">
                <Link href={feature.href}>MORE INFO</Link>
            </Button>
        </div>
    );
}

function FeatureImage({ feature, className }: { feature: Feature; className?: string }) {
    return (
        <div
            className={twMerge(
                "relative min-h-[18rem] overflow-hidden rounded-[var(--radius-panel)] bg-accent-green/40 sm:min-h-[24rem] lg:h-[432px] lg:min-h-0",
                className
            )}
        >
            <Image
                src={feature.image}
                alt={feature.imageAlt}
                fill
                sizes="(min-width: 1024px) 50vw, calc(100vw - 40px)"
                className="object-cover"
            />
        </div>
    );
}
