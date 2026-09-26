import { describe, expect, it } from "vitest";

import { ConfigError } from "../config/api";
import { describeLoadWriteError, isAlreadyGone, isConcurrentChange } from "./loadErrors";

const FAILED = "No se pudo asignar.";

describe("describeLoadWriteError", () => {
  it("names a missing permission instead of blaming the connection", () => {
    expect(describeLoadWriteError(new ConfigError(403, "forbidden"), FAILED)).toBe(
      "No tienes permiso para cambiar la carga académica.",
    );
  });

  it("says someone else changed the load at the same time on 409", () => {
    expect(describeLoadWriteError(new ConfigError(409, "conflict"), FAILED)).toBe(
      "Alguien cambió esta carga al mismo tiempo; recargamos la lista, inténtalo de nuevo.",
    );
  });

  it("passes the server's own words through for any other HTTP failure", () => {
    expect(describeLoadWriteError(new ConfigError(400, "subject not in grade"), FAILED)).toBe(
      "No se pudo asignar. subject not in grade",
    );
  });

  it("falls back to the action and a retry when the server said nothing", () => {
    expect(describeLoadWriteError(new ConfigError(500, ""), FAILED)).toBe("No se pudo asignar. Intenta de nuevo.");
  });

  it("keeps the connection advice for a request that never got an answer", () => {
    expect(describeLoadWriteError(new TypeError("Failed to fetch"), FAILED)).toBe(
      "No se pudo asignar. Revisa tu conexión e intenta de nuevo.",
    );
  });
});

describe("isConcurrentChange", () => {
  it("is true only for a 409 from the server", () => {
    expect(isConcurrentChange(new ConfigError(409, ""))).toBe(true);
    expect(isConcurrentChange(new ConfigError(403, ""))).toBe(false);
    expect(isConcurrentChange(new TypeError("x"))).toBe(false);
  });
});

describe("isAlreadyGone", () => {
  it("is true only for a 404 from the server", () => {
    expect(isAlreadyGone(new ConfigError(404, "teaching assignment not found"))).toBe(true);
    expect(isAlreadyGone(new ConfigError(409, ""))).toBe(false);
    expect(isAlreadyGone(new TypeError("x"))).toBe(false);
  });
});
