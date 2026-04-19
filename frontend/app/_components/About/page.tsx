export default function About() {
  return (
    <section className="relative w-full py-16 md:py-24 bg-white overflow-hidden">
      <div 
        className="absolute inset-0 z-0 opacity-10"
        style={{
          backgroundImage: 'radial-gradient(#000 1px, transparent 1px)',
          backgroundSize: '24px 24px',
        }}
      />

      <div className="relative z-10 max-w-6xl mx-auto px-8 space-y-20 md:space-y-32">
        
        <div className="flex flex-col md:flex-row items-center gap-8 md:gap-16">
          <div className="flex-1 space-y-6 text-[#374151]">
            <h2 className="text-4xl md:text-5xl tracking-widest leading-tight" style={{ fontFamily: "ChenYuluoyan" }}>
              點亮特選之路
            </h2>
            <p className="text-lg md:text-xl leading-relaxed text-gray-600 font-sans">
              漢皇重色思傾國，御宇多年求不得。楊家有女初長成，養在深閨人未識。天生麗質難自棄，一朝選在君王側。迴眸一笑百媚生，六宮粉黛無顏色。，溫泉水滑洗凝脂。侍兒扶起嬌無力，始是新承恩澤時。雲鬢花顏金步搖，芙蓉帳暖度春宵。春宵苦短日高起，從此君王不早朝。
            </p>
            <div className="pt-2">
              <button className="bg-[#333333] text-white px-8 py-2 rounded-xl italic tracking-widest text-xl" style={{ fontFamily: "ChenYuluoyan" }}>
                MORE INFO
              </button>
            </div>
          </div>
          <div className="flex-1 w-full">
            <div className="aspect-[4/3] rounded-[2rem] overflow-hidden shadow-sm border-[10px] border-white">
              <img 
                src="https://images.unsplash.com/photo-1618005182384-a83a8bd57fbe?q=80&w=800&auto=format&fit=crop" 
                alt="點亮特選之路" 
                className="w-full h-full object-cover grayscale-[10%]" 
              />
            </div>
          </div>
        </div>

        <div className="flex flex-col md:flex-row-reverse items-center gap-8 md:gap-16">
          <div className="flex-1 space-y-6 text-[#374151]">
            <h2 className="text-4xl md:text-5xl tracking-widest leading-tight" style={{ fontFamily: "ChenYuluoyan" }}>
              只要有才華，<br className="md:hidden" />不必擔心資訊差
            </h2>
            <p className="text-lg md:text-xl leading-relaxed text-gray-600 font-sans">
              特殊選才資源網致力於透過線上資源匯集檢減少特選資源分布不均等其情況，並輔以論壇功能，讓大家有問題都能及時發問。
            </p>
            <div className="pt-2">
              <button className="bg-[#333333] text-white px-8 py-2 rounded-xl italic tracking-widest text-xl" style={{ fontFamily: "ChenYuluoyan" }}>
                MORE INFO
              </button>
            </div>
          </div>
          <div className="flex-1 w-full">
            <div className="aspect-[4/3] rounded-[2rem] overflow-hidden shadow-sm border-[10px] border-white">
              <img 
                src="https://images.unsplash.com/photo-1633167606207-d840b5070fc2?q=80&w=800&auto=format&fit=crop" 
                alt="只要有才華" 
                className="w-full h-full object-cover grayscale-[10%]" 
              />
            </div>
          </div>
        </div>

      </div>
    </section>
  );
}