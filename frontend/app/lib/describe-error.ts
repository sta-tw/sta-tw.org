import { ApiError } from "./api/types";

/** User-facing message for a failed API call (falls back to a generic one). */
export function describeError(cause: unknown): string {
    if (cause instanceof ApiError) return cause.message || `發生錯誤（${cause.code}）`;
    return "發生未知錯誤，請稍後再試。";
}
