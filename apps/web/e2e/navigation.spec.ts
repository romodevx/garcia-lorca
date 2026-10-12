import { expect, test } from "@playwright/test";

import { HOME_GREETING, openModule, signIn } from "./helpers";

/**
 * El botón "atrás" del navegador.
 *
 * Vale la pena decir por qué existe esta suite. Hasta septiembre de 2026 la web
 * entera vivía en "/" con la pantalla guardada en un useState, así que el
 * navegador nunca veía una segunda página: "atrás" sólo podía sacar del sitio,
 * F5 devolvía al tablero y ningún link era compartible.
 *
 * Nada de esto se puede defender en jsdom — no hay historial que recorrer ni
 * recarga que hacer. Sólo un navegador de verdad.
 */
test.describe("la navegación del navegador", () => {
  test("atrás y adelante se mueven dentro del sitio", async ({ page }) => {
    await signIn(page);

    await openModule(page, "Finanzas");
    await expect(page).toHaveURL(/\/finanzas$/);

    await page.goBack();
    await expect(page).toHaveURL(/localhost:\d+\/$/);
    await expect(page.getByRole("heading", { level: 1, name: HOME_GREETING })).toBeVisible();

    await page.goForward();
    await expect(page).toHaveURL(/\/finanzas$/);
    await expect(page.getByRole("heading", { level: 1, name: /Finanzas/i })).toBeVisible();
  });

  test("recargar conserva el lugar", async ({ page }) => {
    await signIn(page);
    await openModule(page, "Finanzas");

    await page.reload();

    await expect(page).toHaveURL(/\/finanzas$/);
    await expect(page.getByRole("heading", { level: 1, name: /Finanzas/i })).toBeVisible();
  });

  test("un link directo abre el módulo", async ({ page }) => {
    await signIn(page);

    await page.goto("/alumnos");

    // El encabezado dice "Estudiantes"; la ruta dice /alumnos. Las dos palabras
    // son del colegio y no tienen por qué coincidir.
    await expect(page.getByRole("heading", { level: 1, name: /Estudiantes/i })).toBeVisible();
  });

  test("una dirección con un dedazo aterriza en el tablero, no en blanco", async ({ page }) => {
    await signIn(page);

    await page.goto("/finansas");

    await expect(page.getByRole("heading", { level: 1, name: HOME_GREETING })).toBeVisible();
  });

  test("las rutas vecinas siguen siendo suyas y no se las come el catch-all", async ({ page }) => {
    const health = await page.request.get("/healthz");
    expect(health.ok(), "el catch-all se comió /healthz").toBeTruthy();

    // /activar es la página de invitación: tiene que seguir siendo ella misma.
    await page.goto("/activar");
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await expect(page.getByRole("button", { name: "Entrar al sistema" })).toHaveCount(0);
  });

  test("el stub muerto de /login ya no existe", async ({ page }) => {
    // Servía "Stub — la autenticación llega con el módulo IAM" a quien tecleara
    // la dirección. Ahora esa ruta cae en el catch-all, que la manda al tablero.
    await signIn(page);
    await page.goto("/login");

    await expect(page.getByText(/stub/i)).toHaveCount(0);
  });
});
