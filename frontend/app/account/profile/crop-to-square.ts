/** Longest edge of the uploaded avatar; larger photos are scaled down so the
 * re-encoded file comfortably fits the 2 MB limit. */
const MAX_EDGE = 1024;

/** Center-crops an image file to 1:1 on a canvas and re-encodes it in the
 * same format (PNG stays PNG, anything else becomes JPEG). An already
 * square image within MAX_EDGE is returned untouched. */
export async function cropToSquare(file: File): Promise<File> {
    const bitmap = await createImageBitmap(file);
    try {
        const side = Math.min(bitmap.width, bitmap.height);
        if (bitmap.width === bitmap.height && side <= MAX_EDGE) return file;
        const edge = Math.min(side, MAX_EDGE);
        const canvas = document.createElement("canvas");
        canvas.width = edge;
        canvas.height = edge;
        const context = canvas.getContext("2d");
        if (!context) throw new Error("canvas unavailable");
        context.drawImage(
            bitmap,
            (bitmap.width - side) / 2,
            (bitmap.height - side) / 2,
            side,
            side,
            0,
            0,
            edge,
            edge
        );
        const type = file.type === "image/png" ? "image/png" : "image/jpeg";
        const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, type, 0.9));
        if (!blob) throw new Error("encode failed");
        const extension = type === "image/png" ? "png" : "jpg";
        return new File([blob], `avatar.${extension}`, { type });
    } finally {
        bitmap.close();
    }
}
