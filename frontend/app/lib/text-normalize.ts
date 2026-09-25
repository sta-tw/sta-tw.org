/**
 * Folds 臺 onto 台 so client-side text search treats the two variants as
 * equivalent (台灣/臺灣, 台北/臺北, ...) — they're the same character in
 * everyday vs. formal usage but distinct codepoints, so a plain string
 * search would otherwise miss half the matches depending on which variant
 * the query or the source text happens to use.
 */
export function normalizeSearchText(value: string): string {
    return value.replace(/臺/g, "台");
}
