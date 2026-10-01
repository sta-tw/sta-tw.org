"use client";

import { Fragment, useEffect, useState } from "react";
import type { ReactNode } from "react";

import { getSitePolicies, type SitePolicyKey } from "../lib/api/site-policies";
import {
    parsePolicyMarkdown,
    policyDocumentToMarkdown,
    type PolicyBlock,
    type PolicyDocumentContent
} from "../lib/site-policies";

function renderInline(text: string): ReactNode {
    const tokens = text.split(/(\*\*[^*]+\*\*|\x60[^\x60]+\x60|\[[^\]]+\]\([^)]+\))/g);

    return tokens.map((token, index) => {
        if (!token) {
            return null;
        }

        const boldMatch = token.match(/^\*\*(.+)\*\*$/);
        if (boldMatch) {
            return (
                <strong key={index} className="font-semibold text-ink">
                    {boldMatch[1]}
                </strong>
            );
        }

        const codeMatch = token.match(/^\x60(.+)\x60$/);
        if (codeMatch) {
            return (
                <code key={index} className="rounded bg-ink/5 px-1.5 py-0.5 text-[0.9em]">
                    {codeMatch[1]}
                </code>
            );
        }

        const linkMatch = token.match(/^\[([^\]]+)\]\(([^)]+)\)$/);
        if (linkMatch) {
            return (
                <a
                    key={index}
                    href={linkMatch[2]}
                    className="underline decoration-ink/30 underline-offset-4 transition-colors hover:text-ink"
                >
                    {linkMatch[1]}
                </a>
            );
        }

        return <Fragment key={index}>{token}</Fragment>;
    });
}

function renderBlock(block: PolicyBlock, key: string): ReactNode {
    switch (block.type) {
        case "paragraph":
            return (
                <p key={key} className="mb-6 last:mb-0">
                    {renderInline(block.text)}
                </p>
            );
        case "subheading":
            return (
                <h3
                    key={key}
                    className="mt-8 mb-3 text-xl leading-tight font-medium text-ink first:mt-0 sm:text-2xl"
                >
                    {renderInline(block.text)}
                </h3>
            );
        case "unordered-list":
            return (
                <ul
                    key={key}
                    className="mb-6 list-disc space-y-2 pl-6 marker:text-ink/60 last:mb-0"
                >
                    {block.items.map((item, index) => (
                        <li key={index}>{renderInline(item)}</li>
                    ))}
                </ul>
            );
        case "ordered-list":
            return (
                <ol
                    key={key}
                    className="mb-6 list-decimal space-y-2 pl-6 marker:text-ink/60 last:mb-0"
                >
                    {block.items.map((item, index) => (
                        <li key={index}>{renderInline(item)}</li>
                    ))}
                </ol>
            );
    }
}

function PolicyDocumentView({
    document,
    compact = false
}: {
    document: PolicyDocumentContent;
    compact?: boolean;
}) {
    return (
        <article
            className={
                compact
                    ? "w-full"
                    : "mx-auto w-full max-w-screen-lg px-5 pt-12 pb-16 sm:px-6 sm:pt-16 sm:pb-20 lg:px-16 lg:pt-20 lg:pb-24"
            }
        >
            <header
                className={
                    compact
                        ? "border-b border-ink/10 pb-4"
                        : "border-b border-ink/10 pb-8 text-center sm:pb-10"
                }
            >
                <h1
                    className={
                        compact
                            ? "font-serif text-2xl leading-tight text-ink"
                            : "font-sans text-3xl leading-tight font-medium tracking-[-0.03em] text-ink sm:text-4xl lg:text-5xl"
                    }
                >
                    {document.title}
                </h1>
                {compact ? (
                    <p className="mt-2 font-sans text-xs text-copy-muted">即時預覽</p>
                ) : (
                    <div className="mt-5 flex flex-wrap items-center justify-center gap-x-5 gap-y-2 font-sans text-sm text-ink/70 sm:text-base">
                        <time dateTime={document.updatedAt}>
                            Update Date : {document.updatedAt.replaceAll("-", " / ")}
                        </time>
                    </div>
                )}
            </header>

            <div
                className={
                    compact
                        ? "mt-6 max-h-[34rem] overflow-y-auto pr-2 font-sans text-sm leading-7 text-ink/85"
                        : "mx-auto mt-10 max-w-3xl font-sans text-base leading-8 text-ink/85 sm:mt-12 sm:text-lg sm:leading-9"
                }
            >
                {document.intro?.map((paragraph, index) => (
                    <p key={index} className="mb-6 last:mb-10">
                        {renderInline(paragraph)}
                    </p>
                ))}
                {document.sections.map((section, sectionIndex) => (
                    <section
                        key={section.heading || sectionIndex}
                        className="mb-10 last:mb-0 sm:mb-12"
                    >
                        {section.heading ? (
                            <h2 className="mb-4 text-2xl leading-tight font-medium text-ink sm:mb-5 sm:text-3xl">
                                {renderInline(section.heading)}
                            </h2>
                        ) : null}
                        {section.blocks.map((block, index) =>
                            renderBlock(block, section.heading + "-" + index)
                        )}
                    </section>
                ))}
                {document.closing?.map((paragraph, index) => (
                    <p key={index} className="mb-6 last:mb-0">
                        {renderInline(paragraph)}
                    </p>
                ))}
            </div>
        </article>
    );
}

export function PolicyMarkdownPreview({
    markdown,
    fallback
}: {
    markdown: string;
    fallback: PolicyDocumentContent;
}) {
    return (
        <div
            aria-label="政策即時預覽"
            className="rounded-[var(--radius-small)] border border-ink/10 bg-white/50 p-4 sm:p-5"
        >
            <PolicyDocumentView document={parsePolicyMarkdown(markdown, fallback)} compact />
        </div>
    );
}

export default function PolicyDocument({
    policy,
    fallback
}: {
    policy: SitePolicyKey;
    fallback: PolicyDocumentContent;
}) {
    const defaultMarkdown = policyDocumentToMarkdown(fallback);
    const [markdown, setMarkdown] = useState(defaultMarkdown);

    useEffect(() => {
        let ignore = false;
        getSitePolicies()
            .then((response) => {
                if (ignore) return;
                const remoteValue = response.data[policy]?.value.trim();
                if (remoteValue) {
                    setMarkdown(response.data[policy].value);
                }
            })
            .catch(() => {
                // The checked-in document remains available if the public
                // settings endpoint is unavailable.
            });
        return () => {
            ignore = true;
        };
    }, [policy]);

    return (
        <main className="article-dots flex-1 bg-surface">
            <PolicyDocumentView document={parsePolicyMarkdown(markdown, fallback)} />
        </main>
    );
}
