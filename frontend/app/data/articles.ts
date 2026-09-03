export type Article = {
    slug: string;
    title: string;
    listingTitle: string;
    summary: string;
    date: string;
    readTime: string;
    accent: "green" | "yellow";
    tags: string[];
    content: ArticleSection[];
};

export type ArticleSection = {
    heading?: string;
    paragraphs?: string[];
    items?: string[];
};

export const articles: Article[] = [
    {
        slug: "what-is-special-admission",
        title: "特殊選才是什麼",
        listingTitle: "特殊選才是什麼",
        summary: "特殊選才是給具備特殊才能與潛力學生的升學選擇，讓你的投入被看見。",
        date: "2026-04-25",
        readTime: "1.5 min",
        accent: "green",
        tags: ["特選心得", "升學管道"],
        content: [
            {
                paragraphs: [
                    "「特殊選才」是台灣的大學多元入學的一種招生管道，目的是讓具有特殊才能、特殊經歷或發展潛力，但未必能透過學測、分科測驗等傳統考試展現實力的學生，有機會進入大學。各大學依據自身需求自主招生，因此每間學校、每個科系的條件可能都有所不同。"
                ]
            }
        ]
    },
    {
        slug: "who-is-special-admission-for",
        title: "適合哪些學生",
        listingTitle: "適合哪些學生",
        summary: "只要你有足以展現特殊才能、經歷或發展潛力的故事，都值得了解這條路。",
        date: "2026-04-25",
        readTime: "1 min",
        accent: "yellow",
        tags: ["特選心得", "升學管道"],
        content: [
            {
                items: [
                    "在資訊、藝術、音樂、體育等領域有突出作品或競賽成績",
                    "國際或全國性競賽得獎者",
                    "科學研究、發明展、專題研究成果優秀",
                    "實驗教育、資優生、自學生",
                    "境外臺生、新住民及其子女等具有不同教育背景者",
                    "在創業、社會服務、公益、領導等方面有特殊表現者"
                ],
                paragraphs: ["簡單來說，就是你認為自己足夠「特殊」，那就非常歡迎來嘗試看看！"]
            }
        ]
    }
];

export function getArticle(slug: string) {
    return articles.find((article) => article.slug === slug);
}
