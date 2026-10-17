/**
 * What a deferred write may ask of fetch. `keepalive` lets a delete committed on
 * `pagehide` outlive the page: the unload cancels a plain fetch (gate B1 on
 * #142). Cookies (credentials: "include") and headers still travel with it,
 * which is why this is fetch and not `sendBeacon`.
 */
export type DeferredInit = Pick<RequestInit, "keepalive">;
