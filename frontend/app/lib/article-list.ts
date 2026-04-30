import { articlesData, type Article, type ArticleSection } from "./articles-data";

export type { Article, ArticleSection };

export type ArticlePreview = Omit<Article, "content">;

export const articleList: ArticlePreview[] = articlesData.map((article) => ({
    id: article.id,
    category: article.category,
    title: article.title,
    excerpt: article.excerpt,
    publishedAt: article.publishedAt,
    readTime: article.readTime,
    imageAlt: article.imageAlt,
    imageSrc: article.imageSrc,
    coverTone: article.coverTone,
    tags: article.tags
}));

export function getArticleById(id: string) {
    return articlesData.find((article) => article.id === id);
}

export function getArticleIds() {
    return articlesData.map((article) => article.id);
}
