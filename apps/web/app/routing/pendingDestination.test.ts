import { afterEach, describe, expect, it } from "vitest";

import { forgetPendingDestination, isInternalPath, rememberPendingDestination, takePendingDestination } from "./pendingDestination";

afterEach(() => {
  forgetPendingDestination();
});

describe("isInternalPath — only an address of this site may be remembered (CC-309)", () => {
  it.each(["/horarios", "/egresos", "/alumnos/abc/editar"])("accepts %s", (path) => {
    expect(isInternalPath(path)).toBe(true);
  });

  it.each([
    "https://evil.example/horarios",
    "//evil.example",
    "/\\evil.example",
    "\\\\evil.example",
    "javascript:alert(1)",
    "horarios",
    "",
    "/horarios\nSet-Cookie",
  ])("refuses %j", (path) => {
    expect(isInternalPath(path)).toBe(false);
  });
});

describe("the pending destination survives a remount of the shell (CC-309)", () => {
  it("gives back, once, the screen of the remembered address", () => {
    rememberPendingDestination("/horarios");
    expect(takePendingDestination()).toEqual({ screen: "schedule" });
    expect(takePendingDestination()).toBeUndefined();
  });

  it("never remembers an outside address", () => {
    rememberPendingDestination("//evil.example");
    expect(takePendingDestination()).toBeUndefined();
  });

  it("refuses an outside address planted in storage by hand", () => {
    window.sessionStorage.setItem("gl.pendingDestination", "https://evil.example");
    expect(takePendingDestination()).toBeUndefined();
  });

  it("does not remember the screens that are already the default landing", () => {
    for (const path of ["/", "/entrar"]) {
      rememberPendingDestination(path);
      expect(takePendingDestination()).toBeUndefined();
    }
  });

  it("forgets on demand", () => {
    rememberPendingDestination("/horarios");
    forgetPendingDestination();
    expect(takePendingDestination()).toBeUndefined();
  });
});
