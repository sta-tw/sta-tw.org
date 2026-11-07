"use client";

import { useState, type FormEvent } from "react";
import Button from "../../components/button";
import Field, { FieldGroup } from "../../components/field";
import FileUpload from "../../components/file-upload";
import Select from "../../components/select";
import { ErrorText } from "../../components/status";
import { ToggleChipGroup } from "../../components/toggle-chip";
import {
    VERIFICATION_DOCUMENT_ACCEPT,
    VERIFICATION_DOCUMENT_MAX_BYTES,
    submitDocumentVerification,
    type VerificationDocumentType,
    type VerificationIdentityType,
    type VerificationRequestWithDetails
} from "../../lib/api/verification";
import { useAction } from "../../lib/use-resource";
import SchoolPicker, {
    findSchoolByCode,
    findSchoolByName,
    type SchoolDirectory
} from "../../components/school-picker";
import {
    academicFields,
    academicYearOptions,
    documentTypeOptions,
    identityTypeOptions
} from "./verification-options";

type FieldName = "identity" | "document" | "school" | "year" | "fields" | "file";

const yearOptions = academicYearOptions();
const fieldOptions = academicFields.map((field) => ({ value: field, label: field }));
const ALLOWED_EXTENSIONS = VERIFICATION_DOCUMENT_ACCEPT.split(",");

export default function VerificationForm({
    schools,
    initial,
    replacing,
    onSubmitted,
    onCancel
}: {
    schools: SchoolDirectory;
    /** Prefills the form, e.g. when changing or renewing a submission. */
    initial?: VerificationRequestWithDetails;
    /** A pending request this submission supersedes; withdrawn first. */
    replacing?: string;
    onSubmitted: () => void;
    onCancel?: () => void;
}) {
    const [identityType, setIdentityType] = useState(initial?.identity_type ?? "");
    const [documentType, setDocumentType] = useState(initial?.document_type ?? "");
    const [schoolName, setSchoolName] = useState(
        () => findSchoolByCode(schools.schools, initial?.school_code)?.school_name ?? ""
    );
    const [academicYear, setAcademicYear] = useState(initial ? String(initial.academic_year) : "");
    const [fields, setFields] = useState<string[]>(initial?.academic_fields ?? []);
    const [file, setFile] = useState<File | null>(null);
    const [errors, setErrors] = useState<Partial<Record<FieldName, string>>>({});
    const { pending, error: submitError, run } = useAction();
    const submitting = pending !== null;

    function handleFile(selected: File) {
        const extension = `.${selected.name.split(".").pop()?.toLowerCase()}`;
        let error: string | undefined;
        if (!ALLOWED_EXTENSIONS.includes(extension)) error = "請上傳 PDF、PNG 或 JPG 檔案。";
        else if (selected.size > VERIFICATION_DOCUMENT_MAX_BYTES) error = "檔案需小於 50 MB。";
        setErrors((current) => ({ ...current, file: error }));
        setFile(error ? null : selected);
    }

    async function handleSubmit(event: FormEvent) {
        event.preventDefault();
        const school = findSchoolByName(schools.schools, schoolName);
        const next: Partial<Record<FieldName, string>> = {
            identity: identityType ? undefined : "請選擇申請的身分別。",
            document: documentType ? undefined : "請選擇文件類別。",
            school: school ? undefined : "請從清單中選擇學校。",
            year: academicYear ? undefined : "請選擇學年度。",
            fields: fields.length ? undefined : "請至少選擇一個學群。",
            file: file ? undefined : (errors.file ?? "請上傳證明文件。")
        };
        setErrors(next);
        if (Object.values(next).some(Boolean) || !school || !file) return;

        const ok = await run(() =>
            submitDocumentVerification(
                {
                    academic_year: Number(academicYear),
                    school_code: school.school_code,
                    identity_type: identityType as VerificationIdentityType,
                    document_type: documentType as VerificationDocumentType,
                    academic_fields: fields
                },
                file,
                replacing
            )
        );
        if (ok) onSubmitted();
    }

    return (
        <form onSubmit={handleSubmit} noValidate className="flex flex-col gap-6">
            <div className="grid gap-6 md:grid-cols-2">
                <Field label="申請的身分別" required error={errors.identity}>
                    <Select
                        options={identityTypeOptions}
                        placeholder="請選擇"
                        value={identityType}
                        onChange={(event) => setIdentityType(event.target.value)}
                    />
                </Field>
                <Field label="提交文件類別" required error={errors.document}>
                    <Select
                        options={documentTypeOptions}
                        placeholder="請選擇"
                        value={documentType}
                        onChange={(event) => setDocumentType(event.target.value)}
                    />
                </Field>
                <Field
                    label="錄取學校"
                    required
                    error={
                        errors.school ??
                        (schools.error ? `學校清單載入失敗：${schools.error}` : undefined)
                    }
                >
                    <SchoolPicker
                        directory={schools}
                        value={schoolName}
                        onChange={setSchoolName}
                        disabled={!schools.schools}
                    />
                </Field>
                <Field label="學年度" required error={errors.year}>
                    <Select
                        options={yearOptions}
                        placeholder="請選擇"
                        value={academicYear}
                        onChange={(event) => setAcademicYear(event.target.value)}
                    />
                </Field>
            </div>

            <FieldGroup label="學群" note="（可複選）" required error={errors.fields}>
                <ToggleChipGroup options={fieldOptions} value={fields} onValueChange={setFields} />
            </FieldGroup>

            <Field
                label="證明文件"
                required
                hint="PDF、PNG 或 JPG，50 MB 以內。請確認姓名、學校與身分資訊清晰可辨。"
                error={errors.file}
            >
                <FileUpload
                    accept={VERIFICATION_DOCUMENT_ACCEPT}
                    onFileSelected={handleFile}
                    fileName={file?.name}
                    disabled={submitting}
                />
            </Field>

            <div className="flex flex-wrap items-center gap-3">
                <Button type="submit" disabled={submitting}>
                    {submitting ? "提交中…" : "提交審核"}
                </Button>
                {onCancel && (
                    <Button
                        type="button"
                        variant="secondary"
                        disabled={submitting}
                        onClick={onCancel}
                    >
                        取消
                    </Button>
                )}
                <ErrorText>{submitError}</ErrorText>
            </div>
        </form>
    );
}
