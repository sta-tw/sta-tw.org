import Link from "next/link";
import Button from "../components/button";
import { Card } from "../components/card";

export default function PortfolioBrowser() {
    return (
        <main className="article-dots flex-1">
            <div className="mx-auto flex w-full max-w-screen-lg flex-col px-5 py-12 sm:px-6 sm:py-16 lg:px-8">
                <header className="flex flex-col items-center gap-3 border-b border-ink/10 pb-8 text-center sm:pb-10">
                    <h1 className="font-serif text-4xl tracking-[-0.04em] text-ink sm:text-5xl">
                        交換與公開備審
                    </h1>
                    <p className="max-w-xl font-sans text-base text-copy-muted">
                        參考學長姐準備的備審資料，找到適合自己的呈現方式。
                    </p>
                </header>

                <Card className="mt-8 flex flex-col items-start gap-4 sm:mt-10">
                    <div>
                        <h2 className="font-sans text-xl font-medium text-ink">
                            全站備審瀏覽尚未開放
                        </h2>
                        <p className="mt-2 max-w-2xl font-sans text-base leading-relaxed text-copy-muted">
                            目前正式後端支援個人頁面顯示公開備審，以及會員管理自己的檔案版本；全站搜尋與特選生交換瀏覽尚無對應
                            API。
                        </p>
                    </div>
                    <Button asChild size="base">
                        <Link href="/account/portfolio/">管理我的備審</Link>
                    </Button>
                </Card>
            </div>
        </main>
    );
}
