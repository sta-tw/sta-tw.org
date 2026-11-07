import { Fragment, type ReactNode } from "react";

// A deliberately small Markdown subset for user-written text (profile bios): paragraphs, line
// breaks, - / 1. lists, **bold**, *italic*, `code` and [links](https://…).
// Everything else renders as plain text, and React escapes all of it.

const INLINE = /(\*\*[^*]+\*\*|\*[^*]+\*|`[^`]+`|\[[^\]]+\]\([^)\s]+\))/g;

function renderInline(text: string): ReactNode[] {
    return text.split(INLINE).map((token, index) => {
        if (!token) return null;
        let match = token.match(/^\*\*(.+)\*\*$/);
        if (match) return <strong key={index}>{match[1]}</strong>;
        match = token.match(/^\*(.+)\*$/);
        if (match) return <em key={index}>{match[1]}</em>;
        match = token.match(/^`(.+)`$/);
        if (match)
            return (
                <code key={index} className="rounded bg-ink/5 px-1.5 py-0.5 text-[0.9em]">
                    {match[1]}
                </code>
            );
        match = token.match(/^\[([^\]]+)\]\(([^)\s]+)\)$/);
        if (match && /^https?:\/\//i.test(match[2]))
            return (
                <a
                    key={index}
                    href={match[2]}
                    target="_blank"
                    rel="noopener noreferrer nofollow"
                    className="underline decoration-ink/30 underline-offset-4 hover:decoration-ink"
                >
                    {match[1]}
                </a>
            );
        return <Fragment key={index}>{token}</Fragment>;
    });
}

type Block = { kind: "p"; lines: string[] } | { kind: "ul" | "ol"; items: string[] };

function parseBlocks(markdown: string): Block[] {
    const blocks: Block[] = [];
    for (const raw of markdown.replace(/\r\n?/g, "\n").split("\n")) {
        const line = raw.trim();
        const last = blocks.at(-1);
        if (!line) {
            blocks.push({ kind: "p", lines: [] });
            continue;
        }
        const bullet = line.match(/^[-*]\s+(.+)$/);
        const numbered = line.match(/^\d+[.)]\s+(.+)$/);
        const listKind = bullet ? "ul" : numbered ? "ol" : null;
        if (listKind) {
            const item = (bullet ?? numbered)![1];
            if (last?.kind === listKind) last.items.push(item);
            else blocks.push({ kind: listKind, items: [item] });
        } else if (last?.kind === "p") {
            last.lines.push(line);
        } else {
            blocks.push({ kind: "p", lines: [line] });
        }
    }
    return blocks.filter((block) => (block.kind === "p" ? block.lines.length : block.items.length));
}

export default function Markdown({ markdown }: { markdown: string }) {
    return (
        <div className="flex flex-col gap-3 font-sans text-base leading-7 break-words text-ink">
            {parseBlocks(markdown).map((block, index) => {
                if (block.kind === "p")
                    return (
                        <p key={index}>
                            {block.lines.map((line, lineIndex) => (
                                <Fragment key={lineIndex}>
                                    {lineIndex > 0 && <br />}
                                    {renderInline(line)}
                                </Fragment>
                            ))}
                        </p>
                    );
                const List = block.kind;
                return (
                    <List
                        key={index}
                        className={`${List === "ul" ? "list-disc" : "list-decimal"} space-y-1 pl-6 marker:text-copy-muted`}
                    >
                        {block.items.map((item, itemIndex) => (
                            <li key={itemIndex}>{renderInline(item)}</li>
                        ))}
                    </List>
                );
            })}
        </div>
    );
}
