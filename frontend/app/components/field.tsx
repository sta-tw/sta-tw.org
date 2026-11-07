"use client";

import { Label } from "radix-ui";
import {
    createContext,
    useContext,
    useId,
    type ComponentPropsWithoutRef,
    type ReactNode
} from "react";
import { twMerge } from "tailwind-merge";

type FieldControlProps = Pick<
    ComponentPropsWithoutRef<"input">,
    "id" | "required" | "aria-describedby" | "aria-invalid"
>;

const FieldContext = createContext<FieldControlProps | null>(null);

/** Merges the enclosing Field's id / required / aria wiring into a control's
 * own props. Every form control (Input, Textarea, Select, FileUpload) calls
 * this, so Field never has to know which element is focusable. */
export function useFieldControl<P extends FieldControlProps>(props: P): P {
    const field = useContext(FieldContext);
    if (!field) return props;
    const describedBy = [field["aria-describedby"], props["aria-describedby"]]
        .filter(Boolean)
        .join(" ");
    return {
        ...props,
        id: props.id ?? field.id,
        required: props.required ?? field.required,
        "aria-describedby": describedBy || undefined,
        "aria-invalid": props["aria-invalid"] ?? field["aria-invalid"]
    };
}

interface FieldProps {
    label: ReactNode;
    children: ReactNode;
    required?: boolean;
    hint?: ReactNode;
    error?: ReactNode;
    className?: string;
}

/** Label + control + hint/error. The control picks up its wiring through
 * useFieldControl. */
export default function Field({ label, children, required, hint, error, className }: FieldProps) {
    const id = useId();
    const hintId = hint ? `${id}-hint` : undefined;
    const errorId = error ? `${id}-error` : undefined;
    const control: FieldControlProps = {
        id,
        required,
        "aria-describedby": [hintId, errorId].filter(Boolean).join(" ") || undefined,
        "aria-invalid": error ? true : undefined
    };

    return (
        <div className={twMerge("flex min-w-0 flex-col gap-2", className)}>
            <Label.Root htmlFor={id} className="font-sans text-base font-medium text-ink">
                {label}
                {required && (
                    <span aria-hidden className="ml-1 text-danger">
                        *
                    </span>
                )}
            </Label.Root>
            <FieldContext.Provider value={control}>{children}</FieldContext.Provider>
            {hint && (
                <p id={hintId} className="font-sans text-sm text-copy-muted">
                    {hint}
                </p>
            )}
            {error && (
                <p id={errorId} role="alert" className="font-sans text-sm text-danger">
                    {error}
                </p>
            )}
        </div>
    );
}

/** Field for a group of controls (chips, radios, checkboxes): a fieldset
 * whose legend carries the label and required mark, with the hint/error
 * wired to the group via aria-describedby. */
export function FieldGroup({
    label,
    children,
    required,
    hint,
    error,
    note,
    className,
    legendClassName
}: FieldProps & {
    /** Muted text after the label, e.g. "（可複選）" or a "3 / 8" count. */
    note?: ReactNode;
    legendClassName?: string;
}) {
    const id = useId();
    const hintId = hint ? `${id}-hint` : undefined;
    const errorId = error ? `${id}-error` : undefined;
    return (
        <fieldset
            aria-describedby={[hintId, errorId].filter(Boolean).join(" ") || undefined}
            className={twMerge("flex min-w-0 flex-col gap-3", className)}
        >
            <legend
                className={twMerge(
                    "mb-3 font-sans text-base font-medium text-ink",
                    legendClassName
                )}
            >
                {label}
                {required && (
                    <span aria-hidden className="ml-1 text-danger">
                        *
                    </span>
                )}
                {note && <span className="ml-2 font-normal text-copy-muted">{note}</span>}
            </legend>
            {children}
            {hint && (
                <p id={hintId} className="font-sans text-sm text-copy-muted">
                    {hint}
                </p>
            )}
            {error && (
                <p id={errorId} role="alert" className="font-sans text-sm text-danger">
                    {error}
                </p>
            )}
        </fieldset>
    );
}
