"use client";

import { Search, X } from "lucide-react";
import { Form } from "radix-ui";
import IconButton from "./icon-button";

type ArticleSearchBarProps = {
  query: string;
  resultsCount: number;
  totalCount: number;
  onQueryChange: (nextQuery: string) => void;
};

export default function ArticleSearchBar({
  query,
  resultsCount,
  totalCount,
  onQueryChange,
}: ArticleSearchBarProps) {
  return (
    <Form.Root
      role="search"
      onSubmit={(event) => event.preventDefault()}
      className="flex flex-col gap-3"
    >
      <Form.Field name="article-search" className="flex flex-col gap-3">
        <Form.Label className="sr-only">搜尋文章</Form.Label>
        <div className="flex items-center gap-3 rounded-[calc(var(--radius-panel)-0.5rem)] border border-ink/10 bg-white px-4 py-3 shadow-[var(--shadow-card)] sm:px-5 sm:py-4">
          <Search className="h-5 w-5 shrink-0 text-ink/45" strokeWidth={2.25} />

          <Form.Control asChild>
            <input
              type="search"
              value={query}
              enterKeyHint="search"
              placeholder="搜尋備審策略、面試準備、時程安排..."
              className="min-w-0 flex-1 bg-transparent text-base text-ink outline-none placeholder:text-ink/40 sm:text-lg"
              onChange={(event) => onQueryChange(event.target.value)}
            />
          </Form.Control>

          {query ? (
            <IconButton
              type="button"
              label="清除搜尋"
              icon={<X className="h-4 w-4" strokeWidth={2.5} />}
              className="h-10 w-10 rounded-full bg-ink/5 text-ink hover:bg-ink/10"
              onClick={() => onQueryChange("")}
            />
          ) : null}
        </div>
      </Form.Field>

      {query ? (
        <div aria-live="polite" className="text-sm font-medium text-copy-muted">
          顯示 {resultsCount} / {totalCount} 篇文章
        </div>
      ) : null}
    </Form.Root>
  );
}
