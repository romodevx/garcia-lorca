/**
 * @garcia-lorca/api-client — the single entry point app code uses to talk to
 * the gateway. Wraps the Connect transport and re-exports the generated
 * Connect-Query clients/hooks (see ./src/clients.ts for the gen/ts dependency).
 */
export { createTransport, type TransportOptions } from "./src/transport";
export * from "./src/clients";
