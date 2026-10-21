export interface paths {
    "/api/v1/health/live": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["health-live"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/health/ready": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["health-ready"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
}
export type webhooks = Record<string, never>;
export interface components {
    schemas: {
        Error: {
            error: components["schemas"]["ErrorBody"];
        };
        ErrorBody: {
            /** @enum {string} */
            code: "bad_request" | "unauthenticated" | "token_expired" | "invalid_signature" | "invalid_credentials" | "email_not_verified" | "account_blocked" | "forbidden" | "not_found" | "method_not_allowed" | "conflict" | "lesson_overlap" | "lesson_not_live" | "outside_join_window" | "not_editable" | "answer_frozen" | "already_linked" | "student_exists" | "relationship_not_active" | "audience_not_active" | "role_mismatch" | "invitation_invalid" | "invitation_email_mismatch" | "return_not_allowed" | "issue_has_submissions" | "channel_read_only" | "edit_window_passed" | "gone" | "payload_too_large" | "unsupported_media_type" | "validation_failed" | "lesson_in_past" | "tutor_code_invalid" | "token_invalid" | "rate_limited" | "tutor_unavailable" | "internal" | "provider_unavailable" | "service_unavailable" | "quota_exceeded";
            fields?: components["schemas"]["FieldError"][] | null;
            message: string;
            request_id: string;
        };
        FieldError: {
            code: string;
            path: string;
        };
        LiveStatus: {
            /** @enum {string} */
            status: "ok";
        };
        ReadyChecks: {
            /** @enum {string} */
            database: "ok";
        };
        ReadyStatus: {
            checks: components["schemas"]["ReadyChecks"];
            /** @enum {string} */
            status: "ready";
        };
    };
    responses: never;
    parameters: never;
    requestBodies: never;
    headers: never;
    pathItems: never;
}
export type $defs = Record<string, never>;
export interface operations {
    "health-live": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description OK */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["LiveStatus"];
                };
            };
            /** @description Error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    "health-ready": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description OK */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ReadyStatus"];
                };
            };
            /** @description Error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
}
type FlattenedDeepRequired<T> = {
    [K in keyof T]-?: FlattenedDeepRequired<T[K] extends unknown[] | undefined | null ? Extract<T[K], unknown[]>[number] : T[K]>;
};
type ReadonlyArray<T> = [
    Exclude<T, undefined>
] extends [
    unknown[]
] ? Readonly<Exclude<T, undefined>> : Readonly<Exclude<T, undefined>[]>;
export const errorBodyCodeValues: ReadonlyArray<FlattenedDeepRequired<components>["schemas"]["ErrorBody"]["code"]> = ["bad_request", "unauthenticated", "token_expired", "invalid_signature", "invalid_credentials", "email_not_verified", "account_blocked", "forbidden", "not_found", "method_not_allowed", "conflict", "lesson_overlap", "lesson_not_live", "outside_join_window", "not_editable", "answer_frozen", "already_linked", "student_exists", "relationship_not_active", "audience_not_active", "role_mismatch", "invitation_invalid", "invitation_email_mismatch", "return_not_allowed", "issue_has_submissions", "channel_read_only", "edit_window_passed", "gone", "payload_too_large", "unsupported_media_type", "validation_failed", "lesson_in_past", "tutor_code_invalid", "token_invalid", "rate_limited", "tutor_unavailable", "internal", "provider_unavailable", "service_unavailable", "quota_exceeded"];
export const liveStatusStatusValues: ReadonlyArray<FlattenedDeepRequired<components>["schemas"]["LiveStatus"]["status"]> = ["ok"];
export const readyChecksDatabaseValues: ReadonlyArray<FlattenedDeepRequired<components>["schemas"]["ReadyChecks"]["database"]> = ["ok"];
export const readyStatusStatusValues: ReadonlyArray<FlattenedDeepRequired<components>["schemas"]["ReadyStatus"]["status"]> = ["ready"];
