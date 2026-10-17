import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

import { ACCESS_COOKIE, REFRESH_COOKIE, REFRESH_COOKIE_PATH } from "./cookiePaths";

/**
 * The session hint reads cookies the GATEWAY names and scopes. If auth.go
 * renames gl_refresh or moves its Path, the hint silently answers "no session"
 * and a live 24 h session lands on the login screen. This pins the web's copy
 * to the gateway's source, the one place they are declared.
 */
const AUTH_GO = resolve(process.cwd(), "../../services/gateway/internal/handler/auth.go");

function goConst(source: string, name: string): string {
  const match = new RegExp(`\\b${name}\\s*=\\s*"([^"]+)"`).exec(source);
  if (match?.[1] === undefined) throw new Error(`${name} not found in auth.go`);
  return match[1];
}

describe("the session cookies, as the gateway declares them (auth.go)", () => {
  const source = readFileSync(AUTH_GO, "utf8");

  it.each([
    ["accessCookie", ACCESS_COOKIE],
    ["refreshCookie", REFRESH_COOKIE],
    ["refreshPath", REFRESH_COOKIE_PATH],
  ])("%s matches", (name, web) => {
    expect(goConst(source, name)).toBe(web);
  });
});
