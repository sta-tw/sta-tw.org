import { getAdmissionProgram } from "./admissions";
import { mockResponse, mockStore } from "./mock";

/** A program the user marked "我對此學系有興趣". */
export interface SavedProgram {
    program_identifier: string;
    academic_year: number;
    school_name: string;
    admission_program_name: string;
    saved_at: string;
}

const KEY = "saved-programs";
const readAll = () => mockStore.read<SavedProgram[]>(KEY, []);

/** MOCK: GET /api/v1/saved-programs — newest first. */
export function listSavedPrograms(options?: { signal?: AbortSignal }) {
    return mockResponse(() => ({ data: readAll() }), options?.signal);
}

/** MOCK: GET /api/v1/saved-programs/{identifier} */
export function getSavedProgramStatus(identifier: string, options?: { signal?: AbortSignal }) {
    return mockResponse(
        () => ({
            data: { saved: readAll().some((item) => item.program_identifier === identifier) }
        }),
        options?.signal
    );
}

/** MOCK: PUT /api/v1/saved-programs/{identifier} — idempotent. The mock
 * snapshots the program's names with a real GET (no added latency). */
export async function saveProgram(identifier: string) {
    const existing = readAll().find((item) => item.program_identifier === identifier);
    if (existing) return { data: existing };
    const { data: program } = await getAdmissionProgram(identifier);
    const saved: SavedProgram = {
        program_identifier: program.program_identifier,
        academic_year: program.academic_year,
        school_name: program.school_name,
        admission_program_name: program.admission_program_name,
        saved_at: new Date().toISOString()
    };
    mockStore.write(KEY, [saved, ...readAll()]);
    return { data: saved };
}

/** MOCK: DELETE /api/v1/saved-programs/{identifier} — idempotent. */
export function unsaveProgram(identifier: string) {
    return mockResponse(() => {
        mockStore.write(
            KEY,
            readAll().filter((item) => item.program_identifier !== identifier)
        );
    });
}
