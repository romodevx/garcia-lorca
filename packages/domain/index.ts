/**
 * @garcia-lorca/domain — shared zod schemas & validations.
 *
 * These schemas are the cross-surface validation contract for values that are
 * NOT owned by the proto/OpenAPI contract (UI form input, pagination params,
 * branded primitives). Anything the wire contract already defines is consumed
 * from `@garcia-lorca/api-client` (generated types) — never re-declared here.
 */
export * from "./src/ids";
export * from "./src/pagination";
