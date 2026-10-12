import { expect, test } from "@playwright/test";

import { desbordeLateral, signIn } from "./helpers";

/**
 * El sistema en una tablet acostada y en un monitor grande.
 *
 * Jsdom no aplica CSS, así que nada de esto se puede defender en una prueba
 * unitaria: los anchos viven en globals.css y sólo un navegador los resuelve.
 *
 * El teléfono queda fuera a propósito — este sistema se usa en escritorio y en
 * tablet. El almacén de uniformes es la excepción, se trabaja de pie entre los
 * estantes, y tiene su propia suite (narrow.mobile.spec.ts).
 */

const MODULOS = [
  "/",
  "/finanzas",
  "/alumnos",
  "/configuracion",
  "/administracion",
  "/docentes",
  "/calificaciones",
  "/asistencia",
  "/uniformes",
  "/avisos",
  "/pagos",
] as const;

test.describe("la escuela en una tablet acostada", () => {
  test.use({ viewport: { width: 1024, height: 768 } });

  test("ningún módulo se sale de lado", async ({ page }) => {
    await signIn(page);
    for (const ruta of MODULOS) {
      await page.goto(ruta);
      await expect
        .poll(() => desbordeLateral(page), { message: `${ruta} se sale de la tablet` })
        .toBeLessThanOrEqual(1);
    }
  });

  test("ningún texto queda aplastado a un sliver ilegible", async ({ page }) => {
    // Las rejillas de tabla usan unidades `fr`, así que nunca DESBORDAN: se
    // aplastan. Un desborde de cero no prueba que se pueda leer.
    await signIn(page);
    for (const ruta of ["/alumnos", "/finanzas", "/administracion"]) {
      await page.goto(ruta);
      await page.waitForTimeout(600);
      const aplastados = await page.evaluate(() => {
        const out: string[] = [];
        for (const el of document.querySelectorAll("body *")) {
          if (el.children.length > 0) continue;
          const texto = (el.textContent ?? "").trim();
          if (texto.length < 3) continue;
          if (el.scrollWidth > el.clientWidth + 2 && el.clientWidth < 90) {
            out.push(`${texto.slice(0, 30)} (${el.clientWidth}px)`);
          }
        }
        return out;
      });
      expect(aplastados, `${ruta} aplasta texto en una tablet`).toEqual([]);
    }
  });
});

test.describe("la escuela en un monitor grande", () => {
  test.use({ viewport: { width: 2560, height: 1440 } });

  test("el monitor SE APROVECHA, no sólo cabe", async ({ page }) => {
    // Antes eran 1320px fijos: el 52% de un monitor de 2560, con 620px de
    // margen muerto a cada lado.
    await signIn(page);
    await page.goto("/finanzas");
    const ancho = await page.locator(".page").first().evaluate((el) => el.getBoundingClientRect().width);
    expect(ancho).toBeGreaterThan(1700);
  });

  test("el cuerpo de un aviso NO se estira a todo lo ancho", async ({ page }) => {
    // La página usa el monitor entero; la columna que se LEE se acota. Un
    // párrafo de 1800px hace que el ojo pierda el renglón al regresar.
    await signIn(page);
    await page.goto("/avisos");
    const columna = await page.locator(".page-column").first().evaluate((el) => el.getBoundingClientRect().width);
    expect(columna).toBeLessThanOrEqual(960);
  });

  test("sin scroll lateral en 2560", async ({ page }) => {
    await signIn(page);
    await page.goto("/configuracion");
    await expect.poll(() => desbordeLateral(page)).toBeLessThanOrEqual(1);
  });
});
