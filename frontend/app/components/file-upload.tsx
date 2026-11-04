"use client";

import { Upload } from "lucide-react";
import { useRef } from "react";
import Button from "./button";
import { useFieldControl } from "./field";

type FileUploadProps = {
    /** e.g. ".pdf,.png,.jpg" or "image/png,image/jpeg" */
    accept?: string;
    label?: string;
    disabled?: boolean;
    /** Name of the currently selected file, shown beside the button. */
    fileName?: string;
} & (
    | { multiple?: false; onFileSelected: (file: File) => void; onFilesSelected?: never }
    /** Each pick replaces the previous selection, like a native multi-file input. */
    | { multiple: true; onFilesSelected: (files: File[]) => void; onFileSelected?: never }
);

/** Styled trigger for a hidden native file input. Validation (size, type)
 * stays with the caller since limits differ per upload. */
export default function FileUpload({
    accept,
    multiple,
    onFileSelected,
    onFilesSelected,
    label = "選擇檔案",
    disabled,
    fileName
}: FileUploadProps) {
    const inputRef = useRef<HTMLInputElement>(null);
    const fieldProps = useFieldControl({});
    return (
        <div className="flex min-w-0 flex-wrap items-center gap-3">
            <input
                ref={inputRef}
                {...fieldProps}
                type="file"
                accept={accept}
                multiple={multiple}
                disabled={disabled}
                className="sr-only"
                onChange={(event) => {
                    const files = Array.from(event.target.files ?? []);
                    if (multiple) {
                        if (files.length > 0) onFilesSelected(files);
                    } else if (files[0]) {
                        onFileSelected(files[0]);
                    }
                    event.target.value = "";
                }}
            />
            <Button
                type="button"
                variant="secondary"
                size="base"
                disabled={disabled}
                onClick={() => inputRef.current?.click()}
                className="gap-2"
            >
                <Upload aria-hidden className="size-4" />
                {label}
            </Button>
            {fileName && (
                <span className="min-w-0 truncate font-sans text-sm text-copy-muted">
                    {fileName}
                </span>
            )}
        </div>
    );
}
