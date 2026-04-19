"use client";

import { useEffect, useMemo, useState } from "react";

type HttpMethod =
  | "get"
  | "post"
  | "put"
  | "delete"
  | "patch"
  | "options"
  | "head";

type OpenApiOperation = {
  summary?: string;
  operationId?: string;
  tags?: string[];
};

type OpenApiSpec = {
  info?: { title?: string; version?: string };
  paths?: Record<string, Partial<Record<HttpMethod, OpenApiOperation>>>;
};

type Endpoint = {
  method: string;
  path: string;
  summary: string;
  tag: string;
};

const METHOD_ORDER: HttpMethod[] = [
  "get",
  "post",
  "put",
  "patch",
  "delete",
  "options",
  "head",
];

function getBadgeColor(method: string): string {
  switch (method) {
    case "GET":
      return "#166534";
    case "POST":
      return "#1d4ed8";
    case "PUT":
      return "#b45309";
    case "PATCH":
      return "#7c3aed";
    case "DELETE":
      return "#b91c1c";
    default:
      return "#374151";
  }
}

export default function ApiDocsPage() {
  const [spec, setSpec] = useState<OpenApiSpec | null>(null);
  const [error, setError] = useState<string>("");

  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const apiUrl = process.env.NEXT_PUBLIC_API_URL || "http://localhost:12004";
        const response = await fetch(`${apiUrl}/openapi.json`, { cache: "no-store" });
        if (!response.ok) {
          throw new Error(`openapi.json request failed: ${response.status}`);
        }
        const json = (await response.json()) as OpenApiSpec;
        if (!cancelled) {
          setSpec(json);
        }
      } catch (e) {
        if (!cancelled) {
          setError(e instanceof Error ? e.message : "unknown error");
        }
      }
    }
    load();
    return () => {
      cancelled = true;
    };
  }, []);

  const endpoints = useMemo(() => {
    if (!spec?.paths) return [] as Endpoint[];

    const out: Endpoint[] = [];
    for (const path of Object.keys(spec.paths).sort()) {
      const methods = spec.paths[path] || {};
      for (const method of METHOD_ORDER) {
        const op = methods[method];
        if (!op) continue;
        out.push({
          method: method.toUpperCase(),
          path,
          summary: op.summary || op.operationId || "-",
          tag: op.tags?.[0] || "untagged",
        });
      }
    }
    return out;
  }, [spec]);

  const grouped = useMemo(() => {
    const map = new Map<string, Endpoint[]>();
    for (const endpoint of endpoints) {
      const bucket = map.get(endpoint.tag) || [];
      bucket.push(endpoint);
      map.set(endpoint.tag, bucket);
    }
    return Array.from(map.entries()).sort(([a], [b]) => a.localeCompare(b));
  }, [endpoints]);

  return (
    <main className="mx-auto max-w-6xl px-6 py-10">
      <h1 className="text-4xl font-semibold">STA Backend API Docs</h1>
      <p className="mt-3 text-lg text-gray-700">
        本頁面內容即時來自本站後端 OpenAPI 規格（`/openapi.json`）。
      </p>

      <div className="mt-6 rounded-lg border border-gray-200 bg-gray-50 p-4 text-sm text-gray-700">
        <div>Service: {spec?.info?.title || "loading..."}</div>
        <div>Version: {spec?.info?.version || "loading..."}</div>
        <div>Endpoints: {endpoints.length}</div>
      </div>

      {error ? (
        <div className="mt-6 rounded-lg border border-red-200 bg-red-50 p-4 text-red-700">
          無法載入 OpenAPI：{error}
        </div>
      ) : null}

      {!error && !spec ? (
        <div className="mt-6 text-gray-700">讀取 API 規格中...</div>
      ) : null}

      {grouped.map(([tag, list]) => (
        <section key={tag} className="mt-8">
          <h2 className="text-2xl font-semibold">{tag}</h2>
          <div className="mt-3 overflow-hidden rounded-lg border border-gray-200">
            {list.map((item) => (
              <div
                key={`${item.method}-${item.path}`}
                className="grid grid-cols-[110px_1fr_1.2fr] gap-3 border-b border-gray-200 px-4 py-3 last:border-b-0"
              >
                <div>
                  <span
                    className="inline-block rounded px-2 py-1 text-xs font-semibold text-white"
                    style={{ backgroundColor: getBadgeColor(item.method) }}
                  >
                    {item.method}
                  </span>
                </div>
                <code className="break-all text-sm">{item.path}</code>
                <div className="text-sm text-gray-700">{item.summary}</div>
              </div>
            ))}
          </div>
        </section>
      ))}
    </main>
  );
}
