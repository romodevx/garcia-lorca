import { afterEach, describe, expect, it } from "vitest";

import { forgetPendingDestination, isInternalPath, rememberPendingDestination, takePendingDestination } from "./pendingDestination";
import { viewToPath } from "./routes";

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

  // Revisión #146: encoded slashes, dot segments and Unicode inside an id must
  // come back as an address of THIS site, whatever the id holds.
  it.each([
    "/alumnos/%2F%2Fevil.example",
    "/alumnos/..%2F..%2Fx",
    "/alumnos/%E2%80%AEn%C3%AD%C3%B1o-%F0%9F%8C%B1",
    "/alumnos/ni%C3%B1o/editar",
  ])("re-resolves %s to a same-site path", (path) => {
    rememberPendingDestination(path);
    const view = takePendingDestination();
    expect(view).toBeDefined();
    const back = viewToPath(view ?? { screen: "dashboard" });
    expect(isInternalPath(back)).toBe(true);
    expect(back.startsWith("//")).toBe(false);
    expect(new URL(back, "https://colegio.example").origin).toBe("https://colegio.example");
  });

  it("never remembers the forced password change", () => {
    rememberPendingDestination("/cambiar-contrasena");
    expect(takePendingDestination()).toBeUndefined();
  });

  it("forgets on demand", () => {
    rememberPendingDestination("/horarios");
    forgetPendingDestination();
    expect(takePendingDestination()).toBeUndefined();
  });
});
