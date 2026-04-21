const stats = [
  { number: "50+", label: "公開備審" },
  { number: "50+", label: "公開備審" },
  { number: "50+", label: "公開備審" },
];

export default function DataSection() {
  return (
    <section className="relative overflow-hidden bg-white min-h-[530px]">
      <p
        aria-hidden
        className="absolute top-5 left-0 font-serif text-[111px] leading-[145%] text-black/20 whitespace-nowrap pointer-events-none select-none"
      >
        Dream it,
      </p>
      <p
        aria-hidden
        className="absolute left-[40%] font-serif text-[111px] leading-[145%] text-black/20 whitespace-nowrap pointer-events-none select-none"
        style={{ top: 376 }}
      >
        and make it possible.
      </p>

      <div className="relative flex justify-around items-center max-w-[1275px] mx-auto pt-[105px] pb-16">
        {stats.map((stat, i) => (
          <div
            key={i}
            className="relative flex items-center justify-center w-72 h-72 rounded-full bg-[#BADAAF]"
          >
            <div className="absolute w-64 h-64 rounded-full bg-[#D9EDBF]" />
            <div className="relative flex flex-col items-center">
              <span className="font-serif text-[79px] leading-[145%] text-black">{stat.number}</span>
              <span className="font-serif text-3xl leading-[145%] text-black">{stat.label}</span>
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}
