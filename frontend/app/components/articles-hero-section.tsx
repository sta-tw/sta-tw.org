import ArticlesCarousel from "./articles-carousel";

const articleSlides = [
    {
        id: "dream-boat",
        imageSrc: "/articles/dream-boat.svg",
        imageAlt: "夜空與湖面上載著金色翅膀剪影的小船"
    },
    {
        id: "moon-garden",
        imageSrc: "/articles/moon-garden.svg",
        imageAlt: "月光下帶有植物與湖面倒影的夢幻夜景"
    },
    {
        id: "sky-bridge",
        imageSrc: "/articles/sky-bridge.svg",
        imageAlt: "跨越湖面的橋與遠方光點構成的藍綠色夜景"
    },
    {
        id: "sunset-orbit",
        imageSrc: "/articles/sunset-orbit.svg",
        imageAlt: "帶有夕色雲霧與環形天體的幻想風景"
    }
];

export default function ArticlesHeroSection() {
    return (
        <section className="bg-surface" aria-labelledby="articles-hero-heading">
            <h1 id="articles-hero-heading" className="sr-only">
                文章總覽精選輪播
            </h1>
            <ArticlesCarousel slides={articleSlides} />
        </section>
    );
}
