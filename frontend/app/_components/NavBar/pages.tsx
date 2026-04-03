"use client";
import { useState } from "react";

export default function NavBar() {
  const [open, setOpen] = useState(false);

  return (
    <nav className="w-full bg-white shadow-sm">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="flex h-16 items-center justify-between">
          <div className="flex items-center space-x-4">
            <p className="w-8 h-8">✿</p>
            <span className="font-semibold text-lg">S.T.A</span>
            <div className="w-24 h-8 border-2 border-dashed border-gray-400" />
          </div>

          <div className="md:hidden">
            <button
              type="button"
              onClick={() => setOpen(!open)}
              className="inline-flex items-center justify-center rounded-md p-2 text-gray-700 hover:text-gray-900 focus:outline-none focus:ring-0 focus-visible:outline-none focus-visible:ring-0"
              aria-label="Toggle menu"
              style={{ WebkitTapHighlightColor: "transparent" }}
            >
              <svg
                className="h-6 w-6"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
                xmlns="http://www.w3.org/2000/svg"
              >
                {open ? (
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M6 18L18 6M6 6l12 12"
                  />
                ) : (
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M4 6h16M4 12h16M4 18h16"
                  />
                )}
              </svg>
            </button>
          </div>

          <div className="hidden md:flex space-x-6">
            <a href="/articlemain" className="text-gray-700 hover:text-gray-900">
              關於特殊選才
            </a>
            <a href="#" className="text-gray-700 hover:text-gray-900">
              報名簡章
            </a>
            <a href="#" className="text-gray-700 hover:text-gray-900">
              論壇
            </a>
            <a href="#" className="text-gray-700 hover:text-gray-900">
              學習網站
            </a>
          </div>

          <div className="hidden md:block">
            <button className="px-4 py-2 bg-gray-800 text-white rounded-md hover:bg-gray-700">
              登入 | 註冊
            </button>
          </div>
        </div>
      </div>

      {/* mobile menu panel */}
      {open && (
        <div className="md:hidden bg-white border-t">
          <div className="px-2 pt-2 pb-3 space-y-1">
            <a
              href="/articlemain"
              className="block px-3 py-2 rounded-md text-base font-medium text-gray-700 hover:bg-gray-100"
            >
              關於特殊選才
            </a>
            <a
              href="#"
              className="block px-3 py-2 rounded-md text-base font-medium text-gray-700 hover:bg-gray-100"
            >
              報名簡章
            </a>
            <a
              href="#"
              className="block px-3 py-2 rounded-md text-base font-medium text-gray-700 hover:bg-gray-100"
            >
              論壇
            </a>
            <a
              href="#"
              className="block px-3 py-2 rounded-md text-base font-medium text-gray-700 hover:bg-gray-100"
            >
              學習網站
            </a>
            <button className="w-full text-left px-3 py-2 rounded-md text-base font-medium text-gray-800 bg-gray-200 hover:bg-gray-300">
              登入 | 註冊
            </button>
          </div>
        </div>
      )}
    </nav>
  );
}