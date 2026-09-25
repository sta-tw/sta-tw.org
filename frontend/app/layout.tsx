import type { Metadata } from "next";
import { Noto_Sans_TC, Noto_Serif_TC } from "next/font/google";
import "./globals.css";
import SiteChrome from "./site-chrome";

const notoSansTC = Noto_Sans_TC({
    variable: "--font-noto-sans-tc",
    subsets: ["latin"],
    weight: ["400", "500", "700"]
});

const notoSerifTC = Noto_Serif_TC({
    variable: "--font-noto-serif-tc",
    subsets: ["latin"],
    weight: ["400", "600"]
});

export const metadata: Metadata = {
    title: "S.T.A 特殊選才資源網",
    description: "特殊選才資源網 - 提供文章、簡章搜尋、論壇等服務"
};

export default function RootLayout({
    children
}: Readonly<{
    children: React.ReactNode;
}>) {
    return (
        <html
            lang="zh-TW"
            className={`${notoSansTC.variable} ${notoSerifTC.variable} h-full antialiased`}
            suppressHydrationWarning
        >
            {/* suppressHydrationWarning above only covers this <html> tag's own
                attributes: it silences the harmless mismatch some browser
                extensions (e.g. Immersive Translate) cause by injecting a
                data-* attribute here before React hydrates. It does not hide
                real hydration bugs elsewhere in the tree. */}
            <body className="flex min-h-full flex-col" suppressHydrationWarning>
                <SiteChrome>{children}</SiteChrome>
            </body>
        </html>
    );
}
