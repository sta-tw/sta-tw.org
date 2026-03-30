import Link from "next/link";

export default function Banner() {
  return (
    <div className="relative w-full h-[650px] md:h-[450px] flex items-center justify-center overflow-hidden bg-white">
      <div 
        className="absolute inset-0 z-0 opacity-10"
        style={{
          backgroundImage: 'radial-gradient(#000 1px, transparent 1px)',
          backgroundSize: '24px 24px',
        }}
      />

      <div 
        className="absolute -right-24 -top-20 w-[120%] md:w-[600px] aspect-square md:aspect-auto md:h-[550px] bg-[#D1E8CF] rounded-full z-10 md:-right-16 md:top-[-5%] md:opacity-90"
      />

      <div 
        className="absolute -left-32 -bottom-10 w-[110%] md:w-[600px] aspect-square md:aspect-auto md:h-[550px] bg-[#FDE68A] rounded-full z-10 md:-left-20 md:top-[-10%]"
        style={{ borderRadius: '40% 60% 70% 30% / 40% 50% 60% 70%' }}
      />

      <div 
        className="relative z-20 flex flex-col items-center text-[#374151] px-6 w-full text-center"
        style={{ fontFamily: "ChenYuluoyan" }}
      >
        <h1 className="text-6xl md:text-7xl mb-8 md:mb-2 tracking-[0.2em] font-normal leading-tight">
          特殊選才<span className="md:hidden"><br /></span>資源網
        </h1>

        <div className="flex flex-col items-start md:items-center w-fit md:w-full text-4xl md:text-5xl mb-12 md:mb-12 tracking-widest leading-[1.4] md:leading-normal">
          <p className="md:inline">Special</p>
          <p className="md:inline md:ml-3">Talent</p>
          <p className="md:inline md:ml-3">Admission</p>
        </div>

        <Link
          href="/articlemain"
          className="hidden md:block bg-[#333333] text-white px-14 py-5 tracking-widest rounded-xl text-4xl transition-all hover:bg-[#444] hover:scale-105 active:scale-95 shadow-md"
        >
          按下按鈕，前途在手
        </Link>
      </div>
    </div>
  );
}