/**
 * What a deferred write may ask of fetch. `keepalive` lets a delete committed on
 * `pagehide` outlive the page: the unload cancels a plain fetch (gate B1 on
 * #142). Cookies (credentials: "include") and headers still travel with it,
 * which is why this is fetch and not `sendBeacon`.
 *
 * Known limit: with an expired access token the keepalive request gets a 401,
 * and authFetch's refresh-and-retry does not survive the unload. That delete is
 * then lost — never duplicated: the server refused it, so the row is still
 * there the next time the page is opened.
 */
export type DeferredInit = Pick<RequestInit, "keepalive">;
