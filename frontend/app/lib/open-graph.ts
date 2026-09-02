export type ExternalLink = {
    url: string;
    fallbackTitle?: string;
};

export type OpenGraphPreview = {
    url: string;
    title: string;
    description?: string;
    imageUrl?: string;
    siteName: string;
};

const MAX_TEXT_LENGTH = 280;
const FETCH_TIMEOUT_MS = 8_000;

// Links are curated application data, not user input. Keeping this fetch server-side avoids
// browser CORS restrictions and means target pages cannot inject markup into our UI.
export async function getOpenGraphPreviews(links: ExternalLink[]) {
    return Promise.all(links.map((link) => getOpenGraphPreview(link)));
}

async function getOpenGraphPreview(link: ExternalLink): Promise<OpenGraphPreview> {
    const sourceUrl = new URL(link.url);
    const fallback = {
        url: sourceUrl.href,
        title: link.fallbackTitle ?? sourceUrl.hostname,
        siteName: sourceUrl.hostname
    };
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), FETCH_TIMEOUT_MS);

    try {
        const response = await fetch(sourceUrl, {
            headers: {
                Accept: "text/html,application/xhtml+xml",
                "User-Agent": "STA-Link-Preview/1.0"
            },
            next: { revalidate: 86_400 },
            redirect: "follow",
            signal: controller.signal
        });

        if (!response.ok || !response.headers.get("content-type")?.includes("text/html")) {
            return fallback;
        }

        const html = await response.text();
        const metadata = extractMetadata(html);
        const finalUrl = new URL(response.url);

        return {
            url: sourceUrl.href,
            title: metadata.title ?? fallback.title,
            description: metadata.description,
            imageUrl: toHttpUrl(metadata.imageUrl, finalUrl),
            siteName: metadata.siteName ?? finalUrl.hostname
        };
    } catch {
        return fallback;
    } finally {
        clearTimeout(timeout);
    }
}

function extractMetadata(html: string) {
    const meta: Record<string, string> = {};
    const metaTags = html.match(/<meta\b[^>]*>/gi) ?? [];

    for (const tag of metaTags) {
        const attributes = parseAttributes(tag);
        const key = (attributes.property ?? attributes.name)?.toLocaleLowerCase();
        const content = attributes.content;

        if (key && content && !meta[key]) {
            meta[key] = cleanText(content);
        }
    }

    const titleMatch = html.match(/<title\b[^>]*>([\s\S]*?)<\/title>/i);

    return {
        title: meta["og:title"] ?? (titleMatch ? cleanText(titleMatch[1]) : undefined),
        description: meta["og:description"] ?? meta.description,
        imageUrl: meta["og:image:secure_url"] ?? meta["og:image"] ?? meta["twitter:image"],
        siteName: meta["og:site_name"]
    };
}

function parseAttributes(tag: string) {
    const attributes: Record<string, string> = {};
    const attributePattern = /([^\s=/>]+)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>`]+)))?/g;
    let match: RegExpExecArray | null;

    while ((match = attributePattern.exec(tag))) {
        const key = match[1].toLocaleLowerCase();
        const value = match[2] ?? match[3] ?? match[4];

        if (value) attributes[key] = decodeHtmlEntities(value);
    }

    return attributes;
}

function cleanText(value: string) {
    return decodeHtmlEntities(value.replace(/\s+/g, " ").trim()).slice(0, MAX_TEXT_LENGTH);
}

function decodeHtmlEntities(value: string) {
    return value
        .replace(/&amp;/gi, "&")
        .replace(/&quot;/gi, '"')
        .replace(/&#39;|&apos;/gi, "'")
        .replace(/&lt;/gi, "<")
        .replace(/&gt;/gi, ">");
}

function toHttpUrl(value: string | undefined, baseUrl: URL) {
    if (!value) return undefined;

    try {
        const url = new URL(value, baseUrl);
        return url.protocol === "https:" || url.protocol === "http:" ? url.href : undefined;
    } catch {
        return undefined;
    }
}
