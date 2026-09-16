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
    await expect(page.getByRole("dialog")).toBeVisible();

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
    await expect(page.getByRole("dialog")).toBeVisible();

    const cerrar = page.getByRole("button", { name: "Cerrar sesión" });
    const caja = await cerrar.boundingBox();
    expect(caja, "el botón de cerrar sesión existe").not.toBeNull();

    // Quién recibiría el clic en ese punto: el velo, no el botón.
    //
    // El `el?.closest(...) !== null` que había aquí daba TRUE cuando
    // elementFromPoint devolvía null — undefined !== null— así que la prueba
    // pasaba sin que el velo tapara nada. Con `caja` en null el punto era (0,0),
    // que además nunca cae sobre el botón.
    expect(caja, "no se pudo ubicar el botón de cerrar sesión").not.toBeNull();
    const enCima = await page.evaluate(
      ([x, y]) => {
        const el = document.elementFromPoint(x as number, y as number);
        return el !== null && el.closest("[data-testid=drawer-backdrop]") !== null;
      },
      [(caja?.x ?? 0) + (caja?.width ?? 0) / 2, (caja?.y ?? 0) + (caja?.height ?? 0) / 2],
    );
    expect(enCima, "el clic llega al botón: el velo no lo tapa").toBe(true);
  });

  test("el cajón toca el borde derecho y mide lo que mide su forma", async ({ page }) => {
    await signIn(page);
    await page.goto("/configuracion");
    await page.getByRole("button", { name: /Nuevo ciclo/ }).first().click();
    await expect(page.getByRole("dialog")).toBeVisible();

    const caja = await page.getByRole("dialog").boundingBox();
    const vp = page.viewportSize();
    expect(Math.round((caja?.x ?? 0) + (caja?.width ?? 0))).toBe(vp?.width);

    /*
     * Antes esta prueba exigía y=0 y alto = alto del monitor. Era cierto y era
     * el defecto: "Nuevo ciclo escolar" son TRES campos, y estirarlos a toda la
     * pantalla los dejaba flotando arriba de un metro de blanco con el botón de
     * guardar clavado hasta abajo. Ahora la altura la pone el contenido, con
     * techo de 85vh, y el panel queda centrado a lo alto.
     */
    const alto = caja?.height ?? 0;
    expect(alto, "el cajón se volvió a estirar a toda la pantalla")
      .toBeLessThan((vp?.height ?? 0) * 0.9);
    expect(caja?.y ?? 0, "el cajón no está centrado a lo alto").toBeGreaterThan(0);
  });

  test("el tabulador es una página de una sola columna", async ({ page, request }) => {
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

    /*
     * Ya no hay cajón que medir: el tabulador es una PÁGINA.
     *
     * Esta prueba nació midiendo que el cajón grande fuera ancho, porque la
     * forma se capturaba por un túnel de 520px. Ensanchar el cajón sólo movió el
     * problema: para que cupieran los catorce campos, el cuerpo fluía en dos
     * columnas y el corte caía donde alcanzara la altura, de modo que agregar un
     * concepto hacía brincar de columna a una sección entera mientras alguien
     * escribía. Lo que se defiende ahora es que sea página y que su cuerpo sea
     * UNA columna.
     */
    await expect(page.getByRole("heading", { name: /^Precios de / })).toBeVisible();
    await expect(page.getByRole("dialog")).toHaveCount(0);

    const columnas = await page.evaluate(() => {
      const body = document.querySelector(".create-page-body");
      return body === null ? "" : getComputedStyle(body).columnCount;
    });
    expect(columnas, "la forma volvió a fluir en columnas").not.toBe("2");
  });
});
