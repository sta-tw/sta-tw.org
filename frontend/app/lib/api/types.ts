export interface Account {
    id: string;
    username: string;
    identity_status: "temporary" | "student" | "senior";
    account_status: "pending_verification" | "active" | "suspended" | "deleted";
    email_verified: boolean;
}

/** Thrown by apiFetch for any non-2xx response; `code` is the stable
 * machine-readable discriminator from the backend's {error:{code,message}}
 * envelope (see docs/openapi.json in the backend). */
export class ApiError extends Error {
    readonly status: number;
    readonly code: string;

    constructor(status: number, code: string, message: string) {
        super(message);
        this.name = "ApiError";
        this.status = status;
        this.code = code;
    }
}
