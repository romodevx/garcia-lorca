import { globSync, readFileSync } from "node:fs";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Page } from "./Page";

const SCREENS = globSync("app/_screens/**/*.tsx", { cwd: process.cwd() })
  .filter((f) => !f.includes(".test."))
  .map((f) => [f, readFileSync(f, "utf8")] as const);

describe("el ancho de la página", () => {
  it("ninguna pantalla se queda con el ancho pegado en el JSX", () => {
    // Un estilo en línea le gana a cualquier @media. Este repo ya lo aprendió
    // con .topbar-search: "an inline style beats an author rule, so the box kept
    // its desktop width on a 412px screen". Mientras el ancho viviera en el
    // JSX no había responsive posible, sólo media queries que no se aplicaban.
    const offenders = SCREENS
      .filter(([, src]) => /maxWidth:\s*(?:8[2-9]\d|9\d\d|1[0-9]{3})\s*,\s*\n?\s*margin:\s*"0 auto"/.test(src))
      .map(([f]) => f);
    expect(offenders).toEqual([]);
  });

  it("no quedan cinco anchos distintos sin regla", () => {
    // Eran 820 en Avisos, 900 en Ventanilla, 1160 en Asistencia y
    // Calificaciones, 1180 en Uniformes y 1320 en el resto. En un monitor de
    // 2560px, Avisos usaba el 32%.
    const offenders = SCREENS
      .filter(([, src]) => /maxWidth:\s*(?:820|900|1160|1180|1320)\b/.test(src))
      .map(([f]) => f);
    expect(offenders).toEqual([]);
  });
});

describe("Page", () => {
  it("lleva la clase que el CSS sabe medir", () => {
    render(<Page><p>contenido</p></Page>);
    expect(screen.getByText("contenido").parentElement?.className).toBe("page");
  });

  it("la variante de lectura acota SÓLO su columna, no la página", () => {
    // Avisos y Ventanilla: la página usa todo el monitor, pero el texto se
    // acota. Un párrafo de 1800px hace que el ojo pierda el renglón al regresar.
    render(<Page variant="reading"><p>contenido</p></Page>);
    const column = screen.getByText("contenido").parentElement;
    expect(column?.className).toBe("page-column");
    expect(column?.parentElement?.className).toBe("page page-reading");
  });
});
