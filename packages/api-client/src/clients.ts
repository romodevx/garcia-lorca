/**
 * Re-export point for the GENERATED Connect-Query clients & hooks.
 *
 * ┌─────────────────────────────────────────────────────────────────────────┐
 * │ DEPENDENCY: this file consumes `gen/ts`, produced by `buf generate` from  │
 * │ proto/ (Protobuf-ES + Connect-Query/ES). gen/ is gitignored and is        │
 * │ NEVER hand-edited. Until `buf generate` has run, gen/ts is empty and the  │
 * │ re-exports below stay commented out.                                      │
 * └─────────────────────────────────────────────────────────────────────────┘
 *
 * Once generation has run, re-export the per-service query modules here, e.g.:
 *
 *   export * as studentsQuery from "@garcia-lorca/gen/garcialorca/students/v1/students-StudentService_connectquery";
 *   export * as iamQuery from "@garcia-lorca/gen/garcialorca/iam/v1/iam-AuthService_connectquery";
 *
 * Consumers then use the generated `useQuery`/`useMutation` Connect-Query hooks
 * with the transport from `createTransport`. Keeping all re-exports funneled
 * through this one module means app code imports only `@garcia-lorca/api-client`,
 * never deep paths into gen/ts.
 */
export {};
