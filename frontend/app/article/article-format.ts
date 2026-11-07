import { estimateReadingMinutes } from "../lib/api/content";

/** "3 min", the reading-time label used on cards and article headers. */
export function readingTimeOf(body: string) {
    return `${estimateReadingMinutes(body)} min`;
}
