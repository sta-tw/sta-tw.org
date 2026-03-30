"use client";

import { useEffect, useState } from "react";

const articleCards = [
  {
    id: 1,
    title: "什麼是特殊選才？用五分鐘讓你快速了解最小眾的升學渠道！",
    excerpt: "特殊選才是競爭很特殊的選才",
    date: "2026 - 03 - 13",
    tone: "bg-[#ccd7bf]",
  },
  {
    id: 2,
    title: "資訊工程學系的升學需要怎麼準備？學長姐來分享",
    excerpt:
      "喔對這裡是自定義簡介喔對這裡是自定義簡介喔對這裡是自定義簡介喔對這裡是自定義簡介喔對這裡是自定義簡介",
    date: "2026 - 03 - 13",
    tone: "bg-[#e8d9a5]",
  },
];

const tags = [
  {
    text: "# 成大",
    mobileSize: "text-[26px]",
    desktopSize: "md:text-3xl",
    bg: "bg-[#ecd95f]",
    rotate: "-rotate-2",
    desktopPos: "md:left-[19%] md:top-[11%]",
  },
  {
    text: "# 輔導",
    mobileSize: "text-[34px]",
    desktopSize: "md:text-4xl",
    bg: "bg-[#f0c956]",
    rotate: "rotate-2",
    desktopPos: "md:left-[38%] md:top-[15%]",
  },
  {
    text: "# 作品集",
    mobileSize: "text-[28px]",
    desktopSize: "md:text-3xl",
    bg: "bg-[#f3de72]",
    rotate: "-rotate-1",
    desktopPos: "md:left-[64%] md:top-[11%]",
  },
  {
    text: "# 特選心得",
    mobileSize: "text-[24px]",
    desktopSize: "md:text-3xl",
    bg: "bg-[#eef091]",
    rotate: "-rotate-3",
    desktopPos: "md:left-[15%] md:top-[34%]",
  },
  {
    text: "# 面試",
    mobileSize: "text-[38px]",
    desktopSize: "md:text-4xl",
    bg: "bg-[#f3d56d]",
    rotate: "rotate-1",
    desktopPos: "md:left-[82%] md:top-[30%]",
  },
  {
    text: "# 備審",
    mobileSize: "text-[54px]",
    desktopSize: "md:text-6xl",
    bg: "bg-[#f2c24c]",
    rotate: "-rotate-1",
    desktopPos: "md:left-[50%] md:top-[36%]",
  },
  {
    text: "# 員工",
    mobileSize: "text-[40px]",
    desktopSize: "md:text-4xl",
    bg: "bg-[#f0d774]",
    rotate: "rotate-1",
    desktopPos: "md:left-[30%] md:top-[52%]",
  },
  {
    text: "# 不分系",
    mobileSize: "text-[48px]",
    desktopSize: "md:text-5xl",
    bg: "bg-[#f1cc61]",
    rotate: "-rotate-2",
    desktopPos: "md:left-[68%] md:top-[60%]",
  },
  {
    text: "# 美術",
    mobileSize: "text-[30px]",
    desktopSize: "md:text-3xl",
    bg: "bg-[#edf08a]",
    rotate: "rotate-0",
    desktopPos: "md:left-[96%] md:top-[52%]",
  },
  {
    text: "# 中文系",
    mobileSize: "text-[26px]",
    desktopSize: "md:text-3xl",
    bg: "bg-[#eef49f]",
    rotate: "rotate-1",
    desktopPos: "md:left-[48%] md:top-[72%]",
  },
];

const carouselImages = [
  {
    src: "https://images.unsplash.com/photo-1534796636912-3b95b3ab5986?q=80&w=1600&auto=format&fit=crop",
    alt: "星空與小船",
  },
  {
    src: "https://images.unsplash.com/photo-1462331940025-496dfbfc7564?q=80&w=1600&auto=format&fit=crop",
    alt: "宇宙星雲",
  },
  {
    src: "https://images.unsplash.com/photo-1451187580459-43490279c0fa?q=80&w=1600&auto=format&fit=crop",
    alt: "深色銀河",
  },
  {
    src: "https://images.unsplash.com/photo-1419242902214-272b3f66ee7a?q=80&w=1600&auto=format&fit=crop",
    alt: "銀河與星塵",
  },
  {
    src: "https://images.unsplash.com/photo-1470229722913-7c0e2dbbafd3?q=80&w=1600&auto=format&fit=crop",
    alt: "夜空星群",
  },
];

export default function Articlemain() {
  const [currentSlide, setCurrentSlide] = useState(0);
  const totalSlides = carouselImages.length;

  const goPrev = () => {
    setCurrentSlide((prev) => (prev - 1 + totalSlides) % totalSlides);
  };

  const goNext = () => {
    setCurrentSlide((prev) => (prev + 1) % totalSlides);
  };

  useEffect(() => {
    const timer = setInterval(() => {
      setCurrentSlide((prev) => (prev + 1) % totalSlides);
    }, 5000);

    return () => clearInterval(timer);
  }, [totalSlides]);

  return (
    <main className="min-h-screen w-full bg-[#f4f4f4] text-[#262626]">
      <div
        className="w-full pb-14 md:pb-16"
        style={{
          backgroundImage: "radial-gradient(rgba(0,0,0,0.06) 1px, transparent 1px)",
          backgroundSize: "16px 16px",
        }}
      >
        <section className="relative overflow-hidden rounded-b-xl border border-[#d9d9d9] bg-[#102137]">
          <div className="relative h-[180px] w-full md:h-[340px]">
            {carouselImages.map((image, index) => (
              <img
                key={image.src}
                src={image.src}
                alt={image.alt}
                className={`absolute inset-0 h-full w-full object-cover transition-opacity duration-700 ${
                  index === currentSlide ? "opacity-90" : "opacity-0"
                }`}
              />
            ))}
          </div>

          <button
            type="button"
            onClick={goPrev}
            className="absolute left-3 top-1/2 flex h-9 w-9 -translate-y-1/2 items-center justify-center rounded-full bg-[#f3dd8b] text-2xl text-[#3d3d3d] shadow-md md:left-4 md:h-14 md:w-14 md:text-4xl"
          >
            &lt;
          </button>
          <button
            type="button"
            onClick={goNext}
            className="absolute right-3 top-1/2 flex h-9 w-9 -translate-y-1/2 items-center justify-center rounded-full bg-[#f3dd8b] text-2xl text-[#3d3d3d] shadow-md md:right-4 md:h-14 md:w-14 md:text-4xl"
          >
            &gt;
          </button>
          <div className="absolute bottom-3 left-1/2 flex -translate-x-1/2 items-center gap-2 md:bottom-5 md:gap-3">
            {carouselImages.map((image, index) => (
              <button
                key={image.alt}
                type="button"
                onClick={() => setCurrentSlide(index)}
                className={`h-2.5 w-2.5 rounded-full md:h-4 md:w-4 ${
                  index === currentSlide ? "bg-[#f2c65d]" : "bg-[#d9d9d9]"
                }`}
                aria-label={`切換到第 ${index + 1} 張`}
              />
            ))}
          </div>
        </section>

        <section className="px-4 pt-5 md:px-6 md:pt-6">
          <div className="mx-auto flex max-w-[980px] items-center rounded-xl border-2 border-[#eab94b] bg-[#f6df93] px-3 py-2 shadow-sm md:px-4 md:py-3">
            <span className="mr-2 text-2xl md:mr-4 md:text-4xl">⌕</span>
            <input
              type="text"
              placeholder='嘗試搜尋「特選心得」'
              className="w-full bg-transparent text-xl placeholder:text-[#8f8a77] focus:outline-none md:text-3xl"
            />
          </div>
        </section>

        <section className="relative mt-6 space-y-5 px-4 md:mt-8 md:space-y-8 md:px-6">
          {articleCards.map((item, index) => (
            <article
              key={item.id}
              className="rounded-xl border border-[#e2e2e2] bg-[#ececec] p-4 shadow-sm md:p-6"
            >
              <div className="flex flex-col gap-4 md:flex-row md:gap-6">
                <div className={`h-28 ${item.tone} rounded-xl md:h-40 md:w-[280px]`} />

                <div className="flex-1">
                  <h2 className="text-[30px] leading-tight tracking-wide md:text-5xl">{item.title}</h2>
                  <p className="mt-2 font-sans text-sm leading-relaxed text-[#343434] md:mt-4 md:text-lg">
                    {item.excerpt}
                  </p>
                  <p className="mt-4 text-right font-sans text-base text-[#4a4a4a] md:mt-8 md:text-3xl">
                    {item.date}
                  </p>
                </div>
              </div>
            </article>
          ))}

        </section>

        <section className="mt-8 px-4 md:mt-12 md:px-6">
          <h3 className="text-[56px] italic leading-none md:pl-6 md:text-6xl">
            Hashtags
          </h3>
          <div className="mt-5 flex flex-wrap items-end justify-center gap-3 md:hidden">
            {tags.map((tag) => (
              <div
                key={`mobile-${tag.text}`}
                className={`w-fit rounded-xl px-3 py-1.5 ${tag.bg} ${tag.rotate} shadow-sm`}
              >
                <span className={`${tag.mobileSize} whitespace-nowrap leading-none`}>{tag.text}</span>
              </div>
            ))}
          </div>

          <div className="mt-8 hidden md:relative md:mx-auto md:block md:h-[340px] md:w-[980px] md:box-border md:px-8">
            {tags.map((tag) => (
              <div
                key={`desktop-${tag.text}`}
                className={`w-fit rounded-xl px-4 py-2 ${tag.bg} ${tag.rotate} ${tag.desktopPos} md:absolute md:-translate-x-1/2 md:-translate-y-1/2 shadow-sm`}
              >
                <span className={`${tag.desktopSize} whitespace-nowrap leading-none`}>{tag.text}</span>
              </div>
            ))}
          </div>
        </section>
      </div>
    </main>
  );
}
