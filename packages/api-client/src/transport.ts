import { createConnectTransport } from "@connectrpc/connect-web";
import type { Transport } from "@connectrpc/connect";

export interface TransportOptions {
  /**
   * Gateway base URL. MUST come from a PUBLIC env var (e.g. NEXT_PUBLIC_API_URL /
   * EXPO_PUBLIC_API_URL) — never an internal/private URL baked into the bundle.
   */
  baseUrl: string;
}

/**
 * Single factory for the Connect transport used by every generated client.
 * The gateway speaks Connect (HTTP/JSON) at the edge; all TS clients are
 * generated into gen/ts (Protobuf-ES + Connect-Query) and share this transport.
 *
 * Auth/credentials are carried by cookies (Secure + HttpOnly + SameSite), so we
 * send credentials with cross-origin requests.
 */
export function createTransport({ baseUrl }: TransportOptions): Transport {
  return createConnectTransport({
    baseUrl,
    fetch: (input, init) => fetch(input, { ...init, credentials: "include" })
  });
}
