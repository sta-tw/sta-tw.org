export default function Info() {
  const stats = [
    { number: "50+", label: "公開備審" },
    { number: "100+", label: "校系簡章" },
    { number: "200+", label: "特選學生" },
  ];

  return (
    <section className="relative w-full py-16 md:py-24 bg-white overflow-hidden">
      <div 
        className="absolute inset-0 z-0 opacity-10"
        style={{
          backgroundImage: 'radial-gradient(#000 1px, transparent 1px)',
          backgroundSize: '24px 24px',
        }}
      />

      <div className="relative z-10 max-w-6xl mx-auto px-8" style={{ fontFamily: "ChenYuluoyan" }}>
        <div className="mb-6 md:mb-16">
          <h2 className="text-5xl md:text-6xl text-gray-300 italic inline-block relative">
            Dream it,
            <div className="absolute -bottom-1 md:bottom-[-8px] left-0 w-full h-[3px] md:h-1"></div>
          </h2>
        </div>

        <div className="flex flex-col md:flex-row justify-around items-center gap-10 md:gap-0">
          {stats.map((item, index) => (
            <div key={index} className="flex flex-col items-center">
              <div className="w-52 h-52 md:w-64 md:h-64 rounded-full bg-[#DCF1D3] border-[10px] md:border-[12px] border-[#CDE5C5] flex flex-col items-center justify-center text-[#374151] shadow-sm">
                <span className="text-5xl md:text-6xl mb-1">{item.number}</span>
                <span className="text-2xl md:text-3xl tracking-widest">{item.label}</span>
              </div>
            </div>
          ))}
        </div>

        <div className="mt-12 md:mt-16 text-right">
          <h2 className="text-4xl md:text-6xl text-gray-300 italic leading-none whitespace-nowrap">
            and make it possible.
          </h2>
        </div>
        
      </div>
    </section>
  );
}