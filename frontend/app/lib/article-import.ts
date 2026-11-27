/** Reads an article written elsewhere (HackMD .md, Word / Google Docs .docx)
 * into the editor's title, body (Markdown) and tags. Runs in the browser;
 * nothing is uploaded. */

export interface ImportedArticle {
    title?: string;
    body: string;
    tags: string[];
}

export const ARTICLE_IMPORT_ACCEPT = {
    markdown: ".md,.markdown,.txt,text/markdown,text/plain",
    docx: ".docx,application/vnd.openxmlformats-officedocument.wordprocessingml.document"
} as const;

/** Imports larger than this are almost certainly not a single article. */
export const ARTICLE_IMPORT_MAX_BYTES = 10 * 1024 * 1024;

export async function importArticleFile(file: File): Promise<ImportedArticle> {
    if (/\.docx$/i.test(file.name)) return importDocx(file);
    return importMarkdown(await file.text());
}

/** HackMD exports start with optional YAML front matter (title, tags) and
 * usually a "# Title" line; both are lifted out of the body. */
export function importMarkdown(source: string): ImportedArticle {
    let text = source.replace(/^﻿/, "").replace(/\r\n?/g, "\n");
    let title: string | undefined;
    let tags: string[] = [];

    const frontMatter = text.match(/^---\n([\s\S]*?)\n---\n?/);
    if (frontMatter) {
        text = text.slice(frontMatter[0].length);
        for (const line of frontMatter[1].split("\n")) {
            const [key, ...rest] = line.split(":");
            const value = rest.join(":").trim();
            if (key.trim() === "title" && value) title = value.replace(/^["']|["']$/g, "");
            if (key.trim() === "tags" && value) {
                tags = value
                    .replace(/^\[|\]$/g, "")
                    .split(",")
                    .map((tag) => tag.trim().replace(/^["']|["']$/g, ""))
                    .filter(Boolean);
            }
        }
    }

    const heading = text.match(/^\s*#\s+(.+)\n?/);
    if (heading) {
        title ??= heading[1].trim();
        text = text.slice(heading[0].length);
    }

    return { title, body: text.trim(), tags };
}

async function importDocx(file: File): Promise<ImportedArticle> {
    // ~100 KB of parser, only needed by the few who import Word files.
    const mammoth = await import("mammoth");
    const { value: html } = await mammoth.convertToHtml(
        { arrayBuffer: await file.arrayBuffer() },
        // Embedded images would become huge data URLs in the body.
        { convertImage: mammoth.images.imgElement(() => Promise.resolve({ src: "" })) }
    );
    const doc = new DOMParser().parseFromString(html, "text/html");
    const blocks: string[] = [];
    let title: string | undefined;

    for (const element of Array.from(doc.body.children)) {
        const tag = element.tagName.toLowerCase();
        const heading = tag.match(/^h([1-6])$/);
        if (heading) {
            const text = inlineMarkdown(element).trim();
            if (!text) continue;
            if (!title && heading[1] === "1") title = text;
            else blocks.push(`${"#".repeat(Number(heading[1]))} ${text}`);
        } else if (tag === "ul" || tag === "ol") {
            const items = Array.from(element.querySelectorAll(":scope > li")).map(
                (item, index) =>
                    `${tag === "ol" ? `${index + 1}.` : "-"} ${inlineMarkdown(item).trim()}`
            );
            if (items.length) blocks.push(items.join("\n"));
        } else {
            const text = inlineMarkdown(element).trim();
            if (text) blocks.push(text);
        }
    }

    // Without a Heading 1, a short first paragraph is most likely the title.
    if (!title && blocks[0] && blocks[0].length <= 50 && !blocks[0].includes("\n")) {
        title = blocks.shift()!.replace(/^[#*\s]+|\*+$/g, "");
    }

    return { title, body: blocks.join("\n\n"), tags: [] };
}

/** Text of an element with bold / italic / links kept as Markdown. */
function inlineMarkdown(node: Node): string {
    if (node.nodeType === Node.TEXT_NODE) return node.textContent ?? "";
    if (!(node instanceof Element)) return "";
    const inner = Array.from(node.childNodes).map(inlineMarkdown).join("");
    switch (node.tagName.toLowerCase()) {
        case "strong":
        case "b":
            return inner.trim() ? `**${inner}**` : inner;
        case "em":
        case "i":
            return inner.trim() ? `*${inner}*` : inner;
        case "a": {
            const href = node.getAttribute("href") ?? "";
            return /^https?:\/\//i.test(href) ? `[${inner}](${href})` : inner;
        }
        case "br":
            return "\n";
        case "img":
            return "";
        default:
            return inner;
    }
}
