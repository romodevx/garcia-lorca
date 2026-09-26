import { describe, expect, it } from "vitest";

import { ConfigError } from "../config/api";
import { describeAccessError } from "./accessErrors";

describe("describeAccessError", () => {
  it("400 shows the server's own words (e.g. the teacher has no email)", () => {
    expect(describeAccessError(new ConfigError(400, "el docente no tiene correo"))).toBe("el docente no tiene correo");
  });

  it("403 says it is a permission matter", () => {
    expect(describeAccessError(new ConfigError(403, "forbidden"))).toBe("No tienes permiso para dar acceso a docentes.");
  });

  it("404 says the teacher is gone", () => {
    expect(describeAccessError(new ConfigError(404, ""))).toBe("Ese docente ya no existe. Recarga la página.");
  });

  it("409 says the account belongs to another teacher", () => {
    expect(describeAccessError(new ConfigError(409, "already linked"))).toBe(
      "Ese correo ya tiene una cuenta ligada a otro docente. Revisa el correo de este docente.",
    );
  });

  it("401 asks to sign in again", () => {
    expect(describeAccessError(new ConfigError(401, ""))).toBe("Tu sesión expiró. Vuelve a entrar e intenta de nuevo.");
  });

  it("a network failure is blamed on the connection, never on the server", () => {
    expect(describeAccessError(new TypeError("Failed to fetch"))).toBe("No se pudo dar el acceso. Revisa tu conexión e intenta de nuevo.");
  });

  it("an empty 400 or an unknown status falls back to a plain retry", () => {
    expect(describeAccessError(new ConfigError(400, ""))).toBe("No se pudo dar el acceso. Intenta de nuevo.");
    expect(describeAccessError(new ConfigError(500, "boom"))).toBe("No se pudo dar el acceso. Intenta de nuevo.");
  });
});
