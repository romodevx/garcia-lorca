import { globSync, readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

/**
 * Un solo cajón en todo el sistema.
 *
 * Llegó a haber NUEVE, ocho de ellos copiados a mano de <SlideOver>, con cinco
 * anchos distintos: 520, 520, 520, 460, 520, 480, 520, 460 y 560. Por eso el
 * ancho del cajón no se podía arreglar en un lugar — no había un lugar. Y por
 * eso el defecto de Configuración (el velo que no cubría la barra superior)
 * pudo existir en una copia mientras las demás se salvaban por casualidad.
 *
 * Esta prueba mira el código fuente porque lo que defiende no es una conducta
 * de una pantalla, sino que NADIE vuelva a escribir la novena copia.
 */
const SCREENS = globSync("app/_screens/**/*.tsx", { cwd: process.cwd() })
  .filter((f) => !f.includes(".test."))
  .map((f) => [f, readFileSync(f, "utf8")] as const);

describe("el cajón", () => {
  it("nadie dibuja el suyo", () => {
    const offenders = SCREENS
      // La firma del velo: fijo, pegado a los cuatro lados, empujando su
      // contenido a la derecha. Un toast fijo abajo al centro no es un cajón.
      .filter(([, src]) => /position:\s*"fixed",\s*inset:\s*0[\s\S]{0,220}justifyContent:\s*"flex-end"/.test(src))
      .map(([f]) => f);
    expect(offenders).toEqual([]);
  });

  it("nadie pega el ancho de un cajón en el JSX", () => {
    // min(520px, 100%) y sus cuatro primos. El ancho vive en globals.css, donde
    // una @media sí puede alcanzarlo.
    const offenders = SCREENS
      .filter(([, src]) => /width:\s*"min\((?:4[0-9]{2}|5[0-9]{2})px/.test(src))
      .map(([f]) => f);
    expect(offenders).toEqual([]);
  });
});
