/** Links to the site's dynamic pages. Every path ends in "/" to match
 * next.config's trailingSlash: true (no redirect hop). */

export { loginHref } from "./login-redirect";

/** A brochure program page. */
export function programHref(identifier: string) {
    return `/bochures/program/${encodeURIComponent(identifier)}/`;
}

/** A published experience article. */
export function experienceHref(id: string) {
    return `/article/experience/?id=${encodeURIComponent(id)}`;
}

/** Someone's public profile; `segment` is a username or custom slug. */
export function userHref(segment: string) {
    return `/user/${encodeURIComponent(segment)}/`;
}

/** The article overview (/articlemain), optionally filtered by tag/search. */
export function articleOverviewHref({ tag, q }: { tag?: string; q?: string } = {}) {
    const params = new URLSearchParams();
    if (tag) params.set("tag", tag);
    if (q) params.set("q", q);
    const query = params.toString();
    return query ? `/articlemain/?${query}` : "/articlemain/";
}

/** The article editor; without an id it starts a new article. */
export function experienceEditorHref(id?: string) {
    return id
        ? `/account/articles/editor/?id=${encodeURIComponent(id)}`
        : "/account/articles/editor/";
}

/** The portfolio editor; without an id it starts a new portfolio. */
export function portfolioEditorHref(id?: string) {
    return id
        ? `/account/portfolio/editor/?id=${encodeURIComponent(id)}`
        : "/account/portfolio/editor/";
}
