import { expect, test, type Page } from "@playwright/test";
import resetLocalDatabase from "./global-setup";
import { signIn, uniqueSuffix } from "./helpers";

/**
 * Configuración académica as "Puesta en marcha" (module_config_setup ON), from
 * an empty school, through the steps' own screens.
 *
 * It runs in its OWN Playwright project (`config-setup`, `pnpm e2e:config-setup`):
 * the gateway reads ENABLED_MODULES at boot, so one stack cannot serve the flag
 * ON and OFF in one pass. The flag is presentation-only, so this spec forces it
 * in the browser by rewriting `/api/v1/features`; the gateway's answer for
 * everything else is untouched.
 *
 * globalSetup empties the database once per run, so this file empties it again
 * in beforeAll with the same local-only reset (it refuses a non-local API_URL
 * and honours E2E_SKIP_RESET=1). Running it WIPES the local database.
 */

const FEATURES = "**/api/v1/features";

async function withSetupView(page: Page): Promise<void> {
  await page.route(FEATURES, async (route) => {
    const response = await route.fetch();
    const flags = (await response.json()) as Record<string, boolean>;
    await route.fulfill({ response, json: { ...flags, module_config_setup: true } });
  });
}

async function openSetup(page: Page): Promise<void> {
  await withSetupView(page);
  await signIn(page);
  await page.getByRole("button", { name: /Configuración/i }).first().click();
  await expect(page.getByRole("heading", { level: 1, name: "Configuración académica" })).toBeVisible();
  await expect(page.getByRole("navigation", { name: /Puesta en marcha|Configuración/ })).toBeVisible();
}

const steps = (page: Page) => page.getByRole("navigation", { name: /Puesta en marcha|Configuración/ });
const step = (page: Page, title: string) => steps(page).getByRole("button", { name: new RegExp(`^${title}`) });

async function openStep(page: Page, title: string): Promise<void> {
  await step(page, title).click();
  await expect(step(page, title)).toHaveAttribute("aria-current", "step");
}

test.describe.serial("Puesta en marcha (module_config_setup ON) from an empty school", () => {
  const tag = uniqueSuffix();
  const cycle = `Ciclo ${tag}`.toLocaleUpperCase("es-MX");
  const level = `Primaria ${tag}`.toLocaleUpperCase("es-MX");
  const grade = `1° ${tag}`.toLocaleUpperCase("es-MX");

  test.beforeAll(() => {
    resetLocalDatabase();
  });

  test("an empty school opens on the first pending step, with no count cards", async ({ page }) => {
    await openSetup(page);
    await expect(steps(page).locator("[aria-current='step']")).toHaveCount(1);
    await expect(page.getByText("Ciclos", { exact: true })).toHaveCount(0);
    // One way into Migrar alumnos at most: the header button and the card became one tool.
    expect(await page.getByRole("button", { name: /Migrar alumnos/ }).count()).toBeLessThanOrEqual(1);
  });

  test("step 2 · a cycle created and activated turns the step done", async ({ page }) => {
    await openSetup(page);
    await openStep(page, "Ciclo escolar");
    await page.getByRole("button", { name: "Nuevo ciclo" }).click();
    const drawer = page.getByRole("dialog", { name: "Nuevo ciclo escolar" });
    await drawer.getByLabel("Nombre del ciclo", { exact: true }).fill(cycle);
    await drawer.getByLabel("Inicia", { exact: true }).fill("2026-08-31");
    await drawer.getByLabel("Termina", { exact: true }).fill("2027-07-09");
    await drawer.getByRole("button", { name: "Crear ciclo" }).click();
    await page.getByRole("button", { name: `Activar ciclo ${cycle}` }).click();
    await expect(step(page, "Ciclo escolar")).toContainText("activo");
  });

  test("step 3 · the first level and its first grade turn the step done without a reload", async ({ page }) => {
    await openSetup(page);
    await openStep(page, "Niveles y grados");
    await page.getByRole("button", { name: "Nuevo nivel" }).click();
    const drawer = page.getByRole("dialog", { name: "Nuevo nivel educativo" });
    await drawer.getByLabel("Nombre del nivel", { exact: true }).fill(level);
    await drawer.getByRole("button", { name: "Crear nivel" }).click();
    await page.getByRole("button", { name: "Nuevo grado" }).click();
    await page.getByLabel("Nombre del grado", { exact: true }).fill(grade);
    await page.keyboard.press("Enter");
    await expect(step(page, "Niveles y grados")).toContainText("1 nivel · 1 grado");
    await expect(step(page, "Niveles y grados")).toHaveAttribute("aria-current", "step");
  });

  test("step 4 · trimesters are done only once every month of the cycle has one", async ({ page }) => {
    await openSetup(page);
    await openStep(page, "Periodos");
    await page.getByRole("group", { name: "Nivel" }).getByRole("button", { name: level }).click();
    await page.getByRole("button", { name: "Crear trimestres" }).click();
    // Trimesters are born with no months, and the panel flags every month in amber.
    await expect(step(page, "Periodos")).toContainText(`Meses sin periodo en ${level}`);

    // A term takes at most six months, so the school's twelve go four to each
    // trimester, in order — one tap at a time, each saved before the next.
    const firstRow = page.getByRole("button", { name: / en 1\.er trimestre$/ });
    await expect(firstRow.first()).toBeVisible();
    const months = (await firstRow.all()).length;
    const trimesters = ["1.er trimestre", "2.º trimestre", "3.er trimestre"];
    for (let i = 0; i < months; i++) {
      const row = trimesters[Math.floor((i * trimesters.length) / months)] ?? "";
      const cell = page.getByRole("button", { name: new RegExp(` en ${row.replace(".", "\\.")}$`) }).nth(i);
      await cell.click();
      await expect(cell).toHaveAttribute("aria-pressed", "true");
    }
    await expect(step(page, "Periodos")).toContainText("1 nivel con periodos");
  });

  test("step 6 · a group for the grade turns the step done, and its pill counts it", async ({ page }) => {
    await openSetup(page);
    await openStep(page, "Grupos");
    await page.getByRole("group", { name: "Nivel" }).getByRole("button", { name: level }).click();
    await page.getByRole("group", { name: "Grado" }).getByRole("button", { name: new RegExp(grade) }).click();
    await page.getByRole("button", { name: "Nuevo grupo" }).click();
    await page.keyboard.press("Enter");
    await expect(step(page, "Grupos")).toContainText("1 grupo");
    await expect(page.getByRole("group", { name: "Grado" }).getByRole("button", { name: new RegExp(grade) })).toContainText("1 grupo");
  });

  test("step 7 · Definir takes the screen, and Cancelar comes back to step 7", async ({ page }) => {
    await openSetup(page);
    await openStep(page, "Colegiaturas");
    await page.getByRole("button", { name: "Definir" }).click();
    await expect(page.getByRole("heading", { name: new RegExp(`^Precios de ${level}`) })).toBeVisible();
    await expect(page.getByRole("navigation", { name: /Puesta en marcha|Configuración/ })).toHaveCount(0);
    await page.getByRole("button", { name: "Cancelar" }).click();
    await expect(step(page, "Colegiaturas")).toHaveAttribute("aria-current", "step");
  });

  test("the session reopens the step it last had open", async ({ page }) => {
    await openSetup(page);
    await openStep(page, "Calendario");
    await page.reload();
    await expect(step(page, "Calendario")).toHaveAttribute("aria-current", "step");
  });

  test("below 900 px the list folds into a 'Paso N de 8' bar, and nothing scrolls sideways", async ({ page }) => {
    await page.setViewportSize({ width: 820, height: 1000 });
    await withSetupView(page);
    await signIn(page);
    await page.getByRole("button", { name: /Configuración/i }).first().click();
    const bar = page.getByRole("button", { name: /^Paso \d de 8 · / });
    await expect(bar).toHaveAttribute("aria-expanded", "false");
    await bar.click();
    await expect(bar).toHaveAttribute("aria-expanded", "true");
    await page.keyboard.press("Escape");
    await expect(bar).toHaveAttribute("aria-expanded", "false");
    await expect(bar).toBeFocused();
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow).toBeLessThanOrEqual(0);
  });
});
