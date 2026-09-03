const stats = [
    { number: "50+", label: "公開備審" },
    { number: "50+", label: "公開備審" },
    { number: "50+", label: "公開備審" }
];

export default function DataSection() {
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
