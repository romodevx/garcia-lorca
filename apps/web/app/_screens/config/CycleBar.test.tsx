import { describe, expect, it } from "vitest";
import { ConfigError } from "../../config/api";
import { cycleFailureMessage } from "./CycleBar";

// Deliberately its own file: Catalog.test.tsx mocks the whole config/api module,
// and these need the REAL ConfigError.
describe("cycleFailureMessage", () => {
  /**
   * The commonest failure is a name the school already used, and the old message
   * for it was "Intenta de nuevo" — advice a person can follow forever without
   * getting anywhere. Every branch here has to point at something the user can
   * actually change.
   */
  it("names a duplicate instead of suggesting a retry that cannot work", () => {
    const message = cycleFailureMessage(new ConfigError(409, "Conflict"));
    expect(message).toContain("Ya existe un ciclo con ese nombre");
    expect(message).not.toContain("Intenta de nuevo");
  });

  it("points at the dates when they are the problem", () => {
    expect(cycleFailureMessage(new ConfigError(400, "Bad Request"))).toContain("fechas");
  });

  it("says the session expired rather than blaming the form", () => {
    expect(cycleFailureMessage(new ConfigError(401, ""))).toContain("sesión");
  });

  it("says it is a permission when it is", () => {
    expect(cycleFailureMessage(new ConfigError(403, ""))).toContain("permiso");
  });

  it("falls back to something honest for anything else", () => {
    expect(cycleFailureMessage(new Error("network down"))).toContain("conexión");
  });
});
