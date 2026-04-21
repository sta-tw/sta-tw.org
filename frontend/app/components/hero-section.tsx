import Button from "./button";

export default function HeroSection() {
  return (
    <section className="relative overflow-hidden bg-white flex items-center justify-center" style={{ minHeight: "calc(66.667vh - 4rem)" }}>
      {/* Yellow decorative circle — intentionally overflows top-left */}
      <div
        className="absolute rounded-full bg-[#FFE184] pointer-events-none"
        style={{ width: 631, height: 645, left: -73, top: -31 }}
      />

      {/* Green decorative circle — intentionally overflows top-right */}
      <div
        className="absolute rounded-full bg-[#D9EDBF] pointer-events-none"
        style={{ width: 616, height: 630, right: -74, top: -210 }}
      />

      <div className="relative flex flex-col items-center gap-4 py-6">
        <div className="flex flex-col items-center gap-2 text-center">
          <h1 className="font-serif text-[96px] leading-tight text-[#363535]">
            特殊選才資源網
          </h1>
          <p className="font-serif text-[64px] leading-tight text-[#363535]">
            Special Talent Admission
          </p>
        </div>
        <Button className="h-16 w-80 text-2xl">按下按鈕，前途在手</Button>
      </div>
    </section>
  );
}
