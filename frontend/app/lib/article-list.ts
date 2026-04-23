export type ArticlePreview = {
    id: string;
    category: string;
    title: string;
    excerpt: string;
    publishedAt: string;
    readTime: string;
    imageAlt: string;
    imageSrc: string;
    coverTone: string;
    tags: string[];
};

export const articleList: ArticlePreview[] = [
    {
        id: "narrative-portfolio",
        category: "備審策略",
        title: "把經歷寫成評審看得懂的故事：特殊選才自傳整理法",
        excerpt:
            "從活動、作品到轉折動機，拆解如何把零散經歷收束成一條有方向感的申請敘事，讓備審資料不再只是一疊紀錄。",
        publishedAt: "2026.04.18",
        readTime: "8 分鐘",
        imageAlt: "夜空與湖面上載著金色翅膀剪影的小船",
        imageSrc: "/articles/dream-boat.svg",
        coverTone: "from-[#1b3d54] via-[#163346] to-[#0f1f2c]",
        tags: ["自傳", "備審", "申請定位"]
    },
    {
        id: "interview-checklist",
        category: "面試準備",
        title: "面試前七天檢查表：口條、作品集與臨場節奏一次整理",
        excerpt:
            "把面試準備切成一週節奏，從自我介紹、情境題到作品呈現順序，建立可以實際排進行事曆的最後衝刺清單。",
        publishedAt: "2026.04.12",
        readTime: "6 分鐘",
        imageAlt: "月光下帶有植物與湖面倒影的夢幻夜景",
        imageSrc: "/articles/moon-garden.svg",
        coverTone: "from-[#1f4c4b] via-[#275c58] to-[#133738]",
        tags: ["面試", "作品集", "表達訓練"]
    },
    {
        id: "brief-reading",
        category: "資訊整理",
        title: "怎麼讀懂校系簡章：把條件、時程與審查重點轉成待辦",
        excerpt:
            "面對冗長簡章時，先抓審查項目與時間節點，再反推準備順序，讓你知道什麼該先做、什麼可以最後補強。",
        publishedAt: "2026.04.06",
        readTime: "7 分鐘",
        imageAlt: "跨越湖面的橋與遠方光點構成的藍綠色夜景",
        imageSrc: "/articles/sky-bridge.svg",
        coverTone: "from-[#1e4157] via-[#244f67] to-[#183242]",
        tags: ["簡章", "時程規劃", "審查重點"]
    },
    {
        id: "experience-review",
        category: "經驗分享",
        title: "學長姐經驗怎麼看才有用：判斷可參考資訊的三個角度",
        excerpt:
            "同一篇經驗文不一定適合每個人。用背景條件、申請年度與校系差異來過濾資訊，避免把別人的解法照單全收。",
        publishedAt: "2026.03.29",
        readTime: "5 分鐘",
        imageAlt: "帶有夕色雲霧與環形天體的幻想風景",
        imageSrc: "/articles/sunset-orbit.svg",
        coverTone: "from-[#6d4a3c] via-[#9b6b54] to-[#422d2a]",
        tags: ["經驗文", "資訊判讀", "校系比較"]
    },
    {
        id: "timeline-planning",
        category: "申請節奏",
        title: "從暑假開始排進度：特殊選才申請 timeline 實作範例",
        excerpt:
            "如果你知道自己會拖延，就不要只寫目標。這篇示範如何把暑假到面試前的任務拆成週次節點與檢查點。",
        publishedAt: "2026.03.20",
        readTime: "9 分鐘",
        imageAlt: "夜空與湖面上載著金色翅膀剪影的小船",
        imageSrc: "/articles/dream-boat.svg",
        coverTone: "from-[#233b58] via-[#2f4d71] to-[#162338]",
        tags: ["時程安排", "拖延對策", "申請流程"]
    },
    {
        id: "portfolio-selection",
        category: "作品整理",
        title: "作品多不等於作品強：如何挑出真正能支持申請主軸的內容",
        excerpt:
            "整理作品不是把資料塞滿，而是把能證明能力與方向的證據留下來。重點在取捨，而不是堆疊頁數。",
        publishedAt: "2026.03.11",
        readTime: "6 分鐘",
        imageAlt: "月光下帶有植物與湖面倒影的夢幻夜景",
        imageSrc: "/articles/moon-garden.svg",
        coverTone: "from-[#2b4f48] via-[#34685e] to-[#173731]",
        tags: ["作品集", "內容取捨", "主軸整理"]
    }
];
