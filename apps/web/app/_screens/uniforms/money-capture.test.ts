import { describe, expect, it } from "vitest";

import { pesosToCents } from "../../_lib/money";

/**
 * El mismo sobrecobro de cien veces, dos veces más.
 *
 * `TuitionPanel` tenía su propio lector de pesos y leía "1850,50" como
 * $185,050 (corregido en 41c9cc41). Uniformes tenía DOS más, con la misma
 * forma: `Number(x.replace(/[^0-9.]/g, "")) * 100`, que borra toda coma
 * incluida la decimal. Uno fijaba el precio del artículo y el otro el cambio
 * que se le devuelve a un padre en el mostrador.
 *
 * MoneyField no salva de esto: cuando lo escrito no es un monto reconocible,
 * deliberadamente NO lo corrige — deja el texto para que la persona lo vea. El
 * que tiene que negarse es el lector.
 */
describe("el lector permisivo que existía en uniformes", () => {
  const permisivo = (raw: string): number => Math.round(Number(raw.replace(/[^0-9.]/g, "")) * 100);

  it("leía la coma decimal como separador de miles", () => {
    expect(permisivo("1850,50")).toBe(18_505_000); // $185,050 en vez de $1,850.50
  });

  it("el lector estricto se niega en vez de inventar una cifra", () => {
    expect(pesosToCents("1850,50")).toBeNaN();
  });

  it("y sigue aceptando lo que MoneyField sí formatea", () => {
    expect(pesosToCents("1,850.50")).toBe(185_050);
    expect(pesosToCents("650.00")).toBe(65_000);
    expect(pesosToCents("1850")).toBe(185_000);
  });

  it("un campo vacío no vale cero", () => {
    // El permisivo devolvía 0: un artículo gratis, o un cambio de todo el total.
    expect(permisivo("")).toBe(0);
    expect(pesosToCents("")).toBeNaN();
  });
});
