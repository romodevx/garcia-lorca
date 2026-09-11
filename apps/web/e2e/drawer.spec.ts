import { expect, test } from "@playwright/test";

import { API_URL, apiAsOwner, signIn } from "./helpers";

/**
 * El cajón, en el monitor donde se vio el defecto.
 *
 * EL 11 DE SEPTIEMBRE DE 2026, en Configuración a 2560px: el velo empezaba
 * debajo de las tarjetas de estadísticas, terminaba en 1503px y dejaba la barra
 * superior encima, sin atenuar. El cajón no llegaba al borde derecho.
 *
 * La causa era que SlideOver nacía dentro de un <div className="fade-up">, y
 * esa animación de `transform` crea un bloque contenedor para los descendientes
 * `position: fixed` — así que `inset: 0` dejó de significar "el viewport".
 *
 * Ninguna prueba unitaria podía atraparlo: jsdom no aplica CSS ni calcula
 * bloques contenedores. Sólo un navegador de verdad.
 */
test.describe("el cajón cubre la pantalla", () => {
  test.use({ viewport: { width: 2560, height: 1440 } });

  test("el velo cubre el viewport ENTERO, barra superior incluida", async ({ page }) => {
    await signIn(page);
    await page.goto("/configuracion");
    await page.getByRole("button", { name: /Nuevo ciclo/ }).first().click();

    const velo = await page.getByTestId("drawer-backdrop").boundingBox();
    const vp = page.viewportSize();

    expect(velo?.x, "el velo no empieza en el borde izquierdo").toBe(0);
    expect(velo?.y, "el velo deja la barra superior descubierta").toBe(0);
    expect(velo?.width).toBe(vp?.width);
    expect(velo?.height).toBe(vp?.height);
  });

  test("con el cajón abierto no se puede cerrar sesión por encima de él", async ({ page }) => {
    // El defecto no era estético: con el velo cortado, la barra superior seguía
    // recibiendo clics. Un cajón "modal" que deja cerrar sesión no es modal.
    await signIn(page);
    await page.goto("/configuracion");
    await page.getByRole("button", { name: /Nuevo ciclo/ }).first().click();

    const cerrar = page.getByRole("button", { name: "Cerrar sesión" });
    const caja = await cerrar.boundingBox();
    expect(caja, "el botón de cerrar sesión existe").not.toBeNull();

    // Quién recibiría el clic en ese punto: el velo, no el botón.
    const enCima = await page.evaluate(
      ([x, y]) => {
        const el = document.elementFromPoint(x as number, y as number);
        return el?.closest("[data-testid=drawer-backdrop]") !== null;
      },
      [(caja?.x ?? 0) + (caja?.width ?? 0) / 2, (caja?.y ?? 0) + (caja?.height ?? 0) / 2],
    );
    expect(enCima, "el clic llega al botón: el velo no lo tapa").toBe(true);
  });

  test("el cajón toca el borde derecho del monitor", async ({ page }) => {
    await signIn(page);
    await page.goto("/configuracion");
    await page.getByRole("button", { name: /Nuevo ciclo/ }).first().click();

    const caja = await page.getByRole("dialog").boundingBox();
    const vp = page.viewportSize();
    expect(Math.round((caja?.x ?? 0) + (caja?.width ?? 0))).toBe(vp?.width);
    expect(caja?.y, "el cajón no llega hasta arriba").toBe(0);
    expect(Math.round(caja?.height ?? 0), "el cajón no llega hasta abajo").toBe(vp?.height);
  });

  test("la forma grande deja de ser un túnel de 520px", async ({ page, request }) => {
    // El tabulador de colegiaturas —inscripción multi-concepto, tramos de
    // precio, colegiatura, cobranza y cuotas— se capturaba por ahí.
    //
    // La precondición va por API, no por la interfaz: la suite empieza con la
    // base vacía, y llegar a este cajón haciendo clic costaría seis pantallas
    // que no son el sujeto de esta prueba.
    const api = await apiAsOwner(request);
    const sello = Date.now().toString(36).slice(-4);
    await api.post(`${API_URL}/api/v1/config/cycles`, {
      data: { name: `2026-2027 ${sello}`, startsOn: "2026-08-01", endsOn: "2027-06-30" },
    });
    await api.post(`${API_URL}/api/v1/config/levels`, {
      data: { name: `PRIMARIA ${sello}`, sepCode: "PRI", enrollmentGranularity: "cycle" },
    });

    await signIn(page);
    await page.goto("/configuracion");
    await page.getByRole("row", { name: new RegExp(`PRIMARIA ${sello}`) })
      .getByRole("button", { name: /Definir|Editar/ }).click();

    const caja = await page.getByRole("dialog", { name: /^Precios de / }).boundingBox();
    expect(caja?.width, "el cajón grande sigue siendo un túnel").toBeGreaterThan(900);
  });
});
