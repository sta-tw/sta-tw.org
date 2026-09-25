import relatedSitesData from "../data/related-sites.json";

export type RelatedSite = {
    label: string;
    href: string;
};

export const relatedSites = relatedSitesData satisfies RelatedSite[];
