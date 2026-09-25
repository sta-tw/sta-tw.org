import type { Metadata } from "next";

export const metadata: Metadata = {
    title: "隱私權政策 | S.T.A 特殊選才資源網",
    description: "S.T.A 特殊選才資源網如何蒐集、使用與保護你的個人資料，包含第三方登入與 Google 日曆授權的說明。"
};

const UPDATED_AT = "2026-09-23";
const CONTACT_EMAIL = "sta.bochures@googlegroups.com";

type Section = {
    heading: string;
    paragraphs?: string[];
    items?: string[];
};

const sections: Section[] = [
    {
        heading: "適用範圍",
        paragraphs: [
            "本政策說明 S.T.A 特殊選才資源網（以下稱「本站」）在你使用網站、註冊帳號、登入第三方服務或使用相關功能時，如何蒐集、使用、儲存與保護你的個人資料。使用本站即表示你已閱讀並同意本政策的內容。",
            "本站是由學生／校友自發維運的資訊整合平台，非任何學校或政府機關的官方系統；本政策僅適用於本站本身，不涵蓋你點擊外部連結後前往的其他網站（例如各校官方招生網站）。"
        ]
    },
    {
        heading: "我們會蒐集哪些資料",
        paragraphs: ["依你使用的功能不同，本站可能會蒐集以下資料："],
        items: [
            "註冊帳號時填寫的使用者名稱、聯絡 email；若要完成身分驗證，需另外提供學校 email（.edu.tw 網域）",
            "使用 Google 或 Discord 帳號登入／綁定時，該服務提供給本站的基本識別資料（帳號 ID、email、暱稱或大頭貼），僅限本站登入所需的最小範圍",
            "若你主動點擊「新增至日曆」並完成 Google 授權，會取得寫入你自己 Google 日曆的權限——這個授權只用來把招生時程寫進你的日曆，本站不會讀取、修改或刪除你日曆裡原本就有的其他事件，也不會存取這個範圍以外的 Google 服務",
            "你在論壇、心得投稿、留言、客服或申訴表單中主動填寫的內容",
            "登入與使用紀錄，例如登入時間、瀏覽器與裝置資訊、供防止異常登入與濫用行為使用的技術性紀錄（如 IP 位址的雜湊值）",
            "維持登入狀態與防止跨站請求偽造（CSRF）所需的必要 Cookie"
        ]
    },
    {
        heading: "我們如何使用這些資料",
        items: [
            "建立與驗證帳號，讓你能夠登入並使用論壇、簡章查詢、投稿等功能",
            "寄送系統通知，例如帳號驗證信、密碼重設信、你有訂閱或參與項目的狀態更新",
            "偵測與防止濫用行為，例如機器人註冊、洗版、異常登入嘗試",
            "在你主動使用「新增至日曆」功能時，將對應的招生時程寫入你的 Google 日曆",
            "維運、除錯與改善網站功能"
        ]
    },
    {
        heading: "第三方服務",
        paragraphs: [
            "本站使用以下第三方服務協助提供功能，你的資料可能因此傳輸至這些服務，並受其各自的隱私權政策規範：",
        ],
        items: [
            "Google（第三方登入、Google 日曆 API）",
            "Discord（第三方登入、社群通知）",
            "Email 服務商（寄送驗證信、通知信）"
        ]
    },
    {
        heading: "我們不會做的事",
        items: [
            "不會販售、出租你的個人資料給第三方",
            "不會將你的資料用於本政策未說明的廣告投放或行銷追蹤",
            "不會在未經你授權的情況下，存取你 Google 帳號裡本站功能範圍以外的資料"
        ]
    },
    {
        heading: "資料保存與刪除",
        paragraphs: [
            "本站會在你使用服務期間保留必要的帳號與使用資料；若你完成 Google 日曆授權後不再需要這個功能，可以隨時到 Google 帳號的第三方應用程式權限設定中自行撤銷授權。",
            `如果你想要求刪除帳號或所蒐集的個人資料，歡迎來信 ${CONTACT_EMAIL}，我們會在合理時間內處理，但法規要求保留的紀錄（如安全防護所需的稽核紀錄）可能無法立即完全刪除。`
        ]
    },
    {
        heading: "未成年使用者",
        paragraphs: [
            "本站服務對象以高中職學生為主，其中可能包含未滿 18 歲的使用者。若你是法定代理人，對未成年子女使用本站有疑慮，歡迎來信與我們聯絡。"
        ]
    },
    {
        heading: "政策的修改",
        paragraphs: [
            "本站可能不時更新本政策內容，更新後會調整本頁最上方的「更新日期」。若有重大變更，我們會盡力透過站內公告或其他合理方式通知使用者。"
        ]
    },
    {
        heading: "聯絡我們",
        paragraphs: [`如果你對本政策或個人資料的處理方式有任何問題，歡迎來信 ${CONTACT_EMAIL}。`]
    }
];

export default function PrivacyPolicyPage() {
    return (
        <main className="article-dots flex-1 bg-surface">
            <article className="mx-auto w-full max-w-screen-lg px-5 pt-12 pb-16 sm:px-6 sm:pt-16 sm:pb-20 lg:px-16 lg:pt-20 lg:pb-24">
                <header className="border-b border-ink/10 pb-8 text-center sm:pb-10">
                    <h1 className="font-sans text-3xl leading-tight font-medium tracking-[-0.03em] text-ink sm:text-4xl lg:text-5xl">
                        隱私權政策
                    </h1>
                    <div className="mt-5 flex flex-wrap items-center justify-center gap-x-5 gap-y-2 font-sans text-sm text-ink/70 sm:text-base">
                        <time dateTime={UPDATED_AT}>
                            Update Date : {UPDATED_AT.replaceAll("-", " / ")}
                        </time>
                    </div>
                </header>

                <div className="mx-auto mt-10 max-w-3xl font-sans text-base leading-8 text-ink/85 sm:mt-12 sm:text-lg sm:leading-9">
                    {sections.map((section) => (
                        <section key={section.heading} className="mb-10 last:mb-0 sm:mb-12">
                            <h2 className="mb-4 text-2xl leading-tight font-medium text-ink sm:mb-5 sm:text-3xl">
                                {section.heading}
                            </h2>
                            {section.paragraphs?.map((paragraph) => (
                                <p key={paragraph} className="mb-6 last:mb-0">
                                    {paragraph}
                                </p>
                            ))}
                            {section.items && (
                                <ul className="mb-6 list-disc space-y-2 pl-6 marker:text-ink/60 last:mb-0">
                                    {section.items.map((item) => (
                                        <li key={item}>{item}</li>
                                    ))}
                                </ul>
                            )}
                        </section>
                    ))}
                </div>
            </article>
        </main>
    );
}
