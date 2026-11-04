"use client";

import { useId, useMemo } from "react";
import { Input } from "./input";
import { listAllSchools, type AdmissionSchool } from "../lib/api/admissions";
import { describeError } from "../lib/describe-error";
import { useResource, type ResourceState } from "../lib/use-resource";

export interface SchoolDirectory {
    status: ResourceState<AdmissionSchool[]>["status"];
    /** Null until loaded. */
    schools: AdmissionSchool[] | null;
    /** User-facing load error, if any. */
    error: string | null;
}

// The directory is effectively static: share one request across the
// account pages (dropped on failure so a retry refetches).
let schoolsRequest: Promise<AdmissionSchool[]> | null = null;
function loadSchools() {
    schoolsRequest ??= listAllSchools()
        .then(({ data }) => data)
        .catch((cause: unknown) => {
            schoolsRequest = null;
            throw cause;
        });
    return schoolsRequest;
}

/** The school directory for the picker's suggestions and code lookups. */
export function useSchoolDirectory(): SchoolDirectory {
    const { state } = useResource(loadSchools);
    return {
        status: state.status,
        schools: state.status === "ready" ? state.data : null,
        error: state.status === "error" ? describeError(state.error) : null
    };
}

export function findSchoolByName(schools: AdmissionSchool[] | null, name: string) {
    const trimmed = name.trim();
    return schools?.find((school) => school.school_name === trimmed);
}

export function findSchoolByCode(schools: AdmissionSchool[] | null, code?: string) {
    return schools?.find((school) => school.school_code === code);
}

/** Free-text school name with the directory as native <datalist>
 * suggestions: browsers filter them by substring as the user types, and
 * the control stays a plain, accessible text input. Use inside a Field. */
export default function SchoolPicker({
    directory,
    value,
    onChange,
    disabled
}: {
    directory: SchoolDirectory;
    value: string;
    onChange: (name: string) => void;
    disabled?: boolean;
}) {
    const listId = useId();
    const options = useMemo(
        () =>
            directory.schools?.map((school) => (
                <option key={school.school_code} value={school.school_name} />
            )),
        [directory.schools]
    );
    return (
        <>
            <Input
                list={listId}
                value={value}
                onChange={(event) => onChange(event.target.value)}
                placeholder={directory.schools ? "輸入關鍵字搜尋學校" : "正在載入學校清單…"}
                autoComplete="off"
                disabled={disabled}
            />
            <datalist id={listId}>{options}</datalist>
        </>
    );
}
