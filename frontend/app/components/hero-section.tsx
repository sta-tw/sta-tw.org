import Button from "./button";

export default function HeroSection() {
  return (
    <section className="relative flex min-h-[calc(100svh_-_4.5rem)] items-center justify-center overflow-hidden bg-surface px-5 py-16 sm:min-h-[calc(78svh_-_4.5rem)] sm:px-6 lg:min-h-[calc(66.667vh_-_4rem)] lg:px-16">
      {/* Yellow decorative circle - intentionally overflows top-left */}
      <div
        className="pointer-events-none absolute -left-24 -top-20 h-[18rem] w-[18rem] rounded-full bg-accent-yellow sm:-left-28 sm:-top-24 sm:h-[28rem] sm:w-[28rem] lg:-left-[73px] lg:-top-[31px] lg:h-[645px] lg:w-[631px]"
        aria-hidden
      />

      {/* Green decorative circle - intentionally overflows top-right */}
      <div
        className="pointer-events-none absolute -right-28 top-8 h-[17rem] w-[17rem] rounded-full bg-accent-green sm:-right-32 sm:-top-28 sm:h-[27rem] sm:w-[27rem] lg:-right-[74px] lg:-top-[210px] lg:h-[630px] lg:w-[616px]"
        aria-hidden
      />

      <div className="relative z-10 mx-auto flex w-full max-w-6xl flex-col items-center gap-7 py-6 text-center sm:gap-8">
        <div className="flex w-full max-w-[52rem] flex-col items-center gap-3">
          <h1 className="text-balance font-serif text-hero-title text-ink">
            特殊選才資源網
          </h1>
          <p className="max-w-full text-balance font-serif text-hero-subtitle text-ink">
            Special Talent Admission
          </p>
        </div>
        <Button className="h-14 w-full max-w-80 text-lg sm:h-16 sm:text-2xl">
          按下按鈕，前途在手
        </Button>
      </div>
    </section>
  );
}
