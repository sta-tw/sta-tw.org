import Image from "next/image";
import staLogo from "../../sta-logo.png";
import staFooterLogo from "../../sta-logo-nobg.png"

export default function Footer() {
  return (
    <>
      <div className="bg-white border-b py-8">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 mb-4">
          <div className="bg-white rounded-lg shadow-md overflow-hidden flex flex-col sm:flex-row">
            <div className="sm:w-1/3 bg-gray-100 flex items-center justify-center">
              <Image src={staLogo} alt="S.T.A logo" className="w-24 h-24" />
            </div>
            <div className="p-6 flex-1">
              <h2 className="text-4xl md:text-[72px] font-semibold leading-none">加入 116 特選 Discord 群</h2>
              <p className="footer-copy-text mt-2 text-gray-600">
                和 1200+ 位志同道合的人一起聊天、討論
              </p>
              <a
                href="#"
                className="inline-block mt-4 text-blue-600 hover:underline font-sans"
              >
                點我加入 →
              </a>
            </div>
          </div>
        </div>
      </div>

      <footer className="bg-white border-t py-10">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="flex flex-col md:flex-row md:justify-between md:items-start">
            <div className="flex flex-col space-y-2 items-center md:items-start">
              <div className="flex flex-col items-center md:items-start">
                <Image src={staFooterLogo} alt="S.T.A logo" className="w-8 h-8" />
                <span className="font-semibold text-[24px]">S.T.A</span>
              </div>
              <p className="footer-bottom-body-text text-gray-600">
                Special Talent Admission | Information Site
              </p>
            </div>

            <div className="mt-8 w-full md:mt-0 grid grid-cols-1 sm:grid-cols-3 gap-8">
              <div>
                <h3 className="footer-bottom-title-text mb-2 text-gray-800">社群媒體</h3>
                <ul className="footer-bottom-body-text space-y-1 text-gray-600">
                  <li>
                    <a href="#" className="hover:underline">
                      Instagram
                    </a>
                  </li>
                  <li>
                    <a href="#" className="hover:underline">
                      Discord
                    </a>
                  </li>
                </ul>
              </div>
              <div>
                <h3 className="footer-bottom-title-text mb-2 text-gray-800">相關網站</h3>
                <ul className="footer-bottom-body-text space-y-1 text-gray-600">
                  <li>
                    <a href="#" className="hover:underline">
                      綠洲計畫
                    </a>
                  </li>
                  <li>
                    <a href="#" className="hover:underline">
                      長浪計畫
                    </a>
                  </li>
                </ul>
              </div>
              <div>
                <h3 className="footer-bottom-title-text mb-2 text-gray-800">關於本站</h3>
                <ul className="footer-bottom-body-text space-y-1 text-gray-600">
                  <li>
                    <a href="#" className="hover:underline">
                      Github
                    </a>
                  </li>
                  <li>
                    <a href="#" className="hover:underline">
                      Dev.Credit
                    </a>
                  </li>
                </ul>
              </div>
            </div>
          </div>
        </div>
      </footer>
    </>
  );
}
