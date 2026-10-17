import { describe, expect, it } from "vitest";

import { GENERIC_SERVER_FAILURE, spanishServerReason } from "./serverReason";

/**
 * ISSUE-CC-113: the services write their reasons in English, and the panels
 * printed them as they came ("that correction would leave fewer pieces than
 * families are already holding", "user is not a guardian"). A person reading
 * the school system never sees English: known reasons are translated, unknown
 * English is dropped, and Spanish the service already wrote passes through.
 */
describe("spanishServerReason", () => {
  it.each([
    ["that correction would leave fewer pieces than families are already holding", "Ese ajuste dejaría menos piezas de las que ya están apartadas para familias."],
    ["user is not a guardian", "Ese correo no es de un tutor. Para dar acceso a la app, usa el correo de un padre, madre o tutor."],
    ["an adjustment cannot take the count below zero", "El ajuste no puede dejar la existencia por debajo de cero."],
    ["a garment with that code already exists", "Ya existe una prenda con esa clave."],
  ])("traduce %s", (raw, spanish) => {
    expect(spanishServerReason(raw)).toBe(spanish);
  });

  it("no distingue mayúsculas ni el punto final", () => {
    expect(spanishServerReason("User is not a guardian.")).toBe(spanishServerReason("user is not a guardian"));
  });

  it("descarta el inglés que no conoce", () => {
    expect(spanishServerReason("the line is not in a state that can be sold")).toBeUndefined();
    expect(spanishServerReason("forbidden")).toBeUndefined();
  });

  it("deja pasar lo que el servicio ya escribió en español", () => {
    const spanish = "la suma de las formas de pago no coincide con el total de la venta";
    expect(spanishServerReason(spanish)).toBe(spanish);
  });

  it("nada que decir cuando no vino nada", () => {
    expect(spanishServerReason(undefined)).toBeUndefined();
    expect(spanishServerReason("  ")).toBeUndefined();
  });

  it("el genérico está en español", () => {
    expect(GENERIC_SERVER_FAILURE).toMatch(/intenta de nuevo/i);
  });
});
