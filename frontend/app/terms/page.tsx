import type { Metadata } from "next";

export const metadata: Metadata = {
    title: "服務條款 | S.T.A 特殊選才資源網",
    description: "使用 S.T.A 特殊選才資源網前，請詳閱本服務條款，瞭解你的權利義務與本站服務的性質與限制。"
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
        heading: "服務說明",
        paragraphs: [
            "S.T.A 特殊選才資源網（以下稱「本站」）是由學生／校友自發維運的資訊整合平台，提供特殊選才相關的簡章搜尋、招生時程、文章與論壇等內容，協助有興趣的學生更容易找到與整理相關資訊。",
            "本站並非任何學校或政府機關的官方系統，網站上的招生名額、時程、報名資格等資料，來源包含各校公開簡章與社群整理資料，僅供參考；正式且最終的招生規定，一律以各校官方公告的招生簡章為準。若本站資料與官方簡章有出入，請以官方簡章為主，並歡迎來信協助我們更新。"
        ]
    },
    {
        heading: "帳號註冊與使用",
        items: [
            "註冊時請提供真實且可聯絡到你的資料；若使用學校 email 進行身分驗證，該 email 需為你本人所有",
            "你有責任妥善保管帳號密碼，任何透過你帳號進行的操作視為你本人所為；若發現帳號遭盜用，請立即與我們聯絡",
            "本站保留在使用者違反本條款時，暫停或終止其帳號使用權的權利"
        ]
    },
    {
        heading: "禁止行為",
        paragraphs: ["使用本站服務時，你同意不會進行以下行為："],
        items: [
            "冒用他人身分、偽造資料或以不實資訊註冊帳號",
            "發布騷擾、歧視、色情、暴力、廣告垃圾訊息或其他違反公序良俗的內容",
            "上傳侵害他人著作權、商標權或其他智慧財產權的內容",
            "以自動化工具大量註冊帳號、爬取資料或干擾網站正常運作",
            "嘗試未經授權存取本站系統、其他使用者的帳號或資料"
        ]
    },
    {
        heading: "使用者生成內容",
        paragraphs: [
            "你在論壇、文章投稿、留言等功能中發布的內容，著作權仍歸你所有；但你同意授權本站在網站範圍內公開展示、儲存與必要的格式調整，以提供服務。",
            "你對自己發布的內容負完全責任。本站對使用者發布的內容不主動審查，但保留在接獲檢舉或發現違反本條款時，移除相關內容或限制帳號使用的權利。"
        ]
    },
    {
        heading: "第三方登入與 Google 日曆授權",
        paragraphs: [
            "本站提供 Google、Discord 第三方帳號登入，方便你快速註冊與登入；使用第三方登入時，你也需要遵守該第三方服務自身的使用條款。",
            "「新增至日曆」功能需要你另外完成 Google 日曆的授權，本站只會使用這個授權將招生時程寫入你自己的日曆，你可以隨時在 Google 帳號設定中撤銷授權。相關資料處理方式請參閱本站隱私權政策。"
        ]
    },
    {
        heading: "免責聲明",
        paragraphs: [
            "本站盡力確保網站內容的正確性與即時性，但不保證資料完全無誤、完整或即時更新，也不對任何因使用或信賴本站資料所產生的直接或間接損失負責。",
            "本站服務以「現況」提供，不保證服務不中斷、無錯誤，或完全符合你的特定需求。"
        ]
    },
    {
        heading: "服務的變更與終止",
        paragraphs: [
            "本站可能因維運需要，隨時調整、暫停或終止部分或全部服務內容，將盡力提前公告，但不另作個別通知。"
        ]
    },
    {
        heading: "條款的修改",
        paragraphs: [
            "本站可能不時更新本條款內容，更新後會調整本頁最上方的「更新日期」。若你在條款修改後繼續使用本站服務，視為你同意修改後的內容。"
        ]
    },
    {
        heading: "準據法與管轄",
        paragraphs: [
            "本條款之解釋與適用，以及與本條款有關的爭議，均以中華民國法律為準據法，並以台灣台北地方法院為第一審管轄法院。"
        ]
    },
    {
        heading: "聯絡我們",
        paragraphs: [`如果你對本條款有任何問題，歡迎來信 ${CONTACT_EMAIL}。`]
    }
];

export default function TermsOfServicePage() {
    return (
        <main className="article-dots flex-1 bg-surface">
            <article className="mx-auto w-full max-w-screen-lg px-5 pt-12 pb-16 sm:px-6 sm:pt-16 sm:pb-20 lg:px-16 lg:pt-20 lg:pb-24">
                <header className="border-b border-ink/10 pb-8 text-center sm:pb-10">
                    <h1 className="font-sans text-3xl leading-tight font-medium tracking-[-0.03em] text-ink sm:text-4xl lg:text-5xl">
                        服務條款
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
