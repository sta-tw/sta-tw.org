/** Shared display formatters. Dates are pinned to Taiwan time so server and
 * client render the same text. */

type DateInput = Date | string | number;

function dateFormatter(options: Intl.DateTimeFormatOptions) {
    const formatter = new Intl.DateTimeFormat("zh-TW", { ...options, timeZone: "Asia/Taipei" });
    return (value: DateInput) => {
        const date = value instanceof Date ? value : new Date(value);
        return Number.isNaN(date.getTime()) ? String(value) : formatter.format(date);
    };
}

/** "2026年4月25日" */
export const formatDate = dateFormatter({ dateStyle: "medium" });
/** "2026年4月25日 下午3:04" */
export const formatDateTime = dateFormatter({ dateStyle: "medium", timeStyle: "short" });
/** "2026/04/25" — the compact form used on article cards and headers. */
export const formatNumericDate = dateFormatter({
    year: "numeric",
    month: "2-digit",
    day: "2-digit"
});

/** "512 KB" / "3.4 MB" */
export function formatBytes(bytes: number) {
    if (bytes < 1024 * 1024) return `${Math.max(1, Math.round(bytes / 1024))} KB`;
    return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

/** The current ROC (民國) year, e.g. 115 in 2026. */
export function currentRocYear(now = new Date()) {
    return now.getFullYear() - 1911;
}

/** ROC academic-year options from `from` to `to` inclusive, e.g.
 * { value: "115", label: "115 學年度" } with suffix " 學年度". */
export function rocYearOptions({
    from,
    to,
    order = "desc",
    suffix = ""
}: {
    from: number;
    to: number;
    order?: "asc" | "desc";
    suffix?: string;
}) {
    const years = Array.from({ length: Math.max(0, to - from + 1) }, (_, index) =>
        order === "asc" ? from + index : to - index
    );
    return years.map((year) => ({ value: String(year), label: `${year}${suffix}` }));
}
