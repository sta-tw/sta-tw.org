export default function Footer() {
  return (
    <>
      <div className="bg-white border-b py-8">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 mb-4">
          <div className="bg-white rounded-lg shadow-md overflow-hidden flex flex-col sm:flex-row">
            <div className="sm:w-1/3 bg-gray-100 flex items-center justify-center">
              <div className="w-24 h-24 bg-gray-300" />
            </div>
            <div className="p-6 flex-1">
              <h2 className="text-lg font-semibold">加入 116 特選 Discord 群</h2>
              <p className="mt-2 text-gray-600 text-sm font-sans">
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
          <div className="flex flex-col md:flex-row md:justify-between md:items-start space-y-8 md:space-y-0">
            <div className="flex flex-col space-y-2">
              <div className="flex items-center space-x-2">
                <div className="w-8 h-8 bg-gray-200 rounded-full" />
                <span className="font-semibold text-xl">S.T.A</span>
              </div>
              <p className="text-gray-600 text-sm font-sans">
                Special Talent Admission | Information Site
              </p>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-3 gap-8">
              <div>
                <h3 className="text-gray-800 font-semibold mb-2">社群媒體</h3>
                <ul className="space-y-1 text-gray-600 text-sm">
                  <li>
                    <a href="#" className="hover:underline">
                      Instagram
                    </a>
                  </li>
                  <li className="flex items-center space-x-1">
                    <a href="#" className="hover:underline">
                      Discord
                    </a>
                  </li>
                </ul>
              </div>
              <div>
                <h3 className="text-gray-800 font-semibold mb-2">相關網站</h3>
                <ul className="space-y-1 text-gray-600 text-sm">
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
                <h3 className="text-gray-800 font-semibold mb-2">關於本站</h3>
                <ul className="space-y-1 text-gray-600 text-sm">
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
