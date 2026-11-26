/** One-off notices the editor hands back to the article list via ?notice=. */
const notices = {
    saved: "已儲存草稿。",
    submitted: "已送出審核，通過後會公開。"
} as const;

export type ArticlesNotice = keyof typeof notices;

export const articlesNoticeHref = (notice: ArticlesNotice) => `/account/articles/?notice=${notice}`;

export function articlesNoticeText(value: string | null) {
    return value && value in notices ? notices[value as ArticlesNotice] : null;
}
