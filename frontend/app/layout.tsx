import "./globals.css";

// import components
import NavBar from "./_components/NavBar/pages";
import Footer from "./_components/Footer/pages";

import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "116 特殊選才資源網",
  description: "Special Talent Acquisition",
  applicationName: "sta-web",
  authors: [{ name: "Aaron-Kao", url: "https://sta-tw.org" }],
  keywords: ["特殊選才", "特殊選才資源", "STA-Web", "簡章"],
  openGraph: {
    images: [
      {
        url: "/sta.png",
        width: 1200,
        height: 630,
        alt: "STA‑Web Open Graph",
      },
    ],
    locale: "zh-TW",
    type: "website",
  },
  icons: {
    icon: "/sta.ico",
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body>
        <NavBar />
        {children}
        <Footer /> 
      </body>
    </html>
  );
}
