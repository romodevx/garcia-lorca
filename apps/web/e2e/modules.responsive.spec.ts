import { expect, test, type Page } from "@playwright/test";

import { desbordeLateral, forceFlags, signIn } from "./helpers";
import { HOME_ROWS, isReportOnly, rowName } from "./matrix";

/**
 * The other modules of responsive.spec.ts under the refreshed look
 * (module_ui_refresh ON), across PLAN-redesign-home §4.3. Scope is D9:
 * BLOCKING up to 200% zoom; at 300–400% the run only REPORTS, through
 * test.info().annotations, and what fails goes to the backlog as its own task.
 *
 * Configuración with module_config_setup ON below 900px is blocking at every
 * row (E6): its "Paso N de 8" bar must stay in view under the floating capsule.
 */

const MODULES = [
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

const REFRESH = { module_ui_refresh: true } as const;
/** Lets tables and late reads land before the width is measured. */
const SETTLE_MS = 400;
/** Below this width Configuración folds its step list into the "Paso N de 8" bar (setup/layout.ts). */
const SETUP_NARROW_MAX_PX = 899;

async function overflowOf(page: Page, path: string): Promise<number> {
  await page.goto(path);
  await expect(page.getByRole("heading", { level: 1 }).first()).toBeVisible();
  await page.waitForTimeout(SETTLE_MS);
  return desbordeLateral(page);
}

for (const row of HOME_ROWS) {
  test.describe(`look refresh · ${rowName(row)}`, () => {
    test.use({
      viewport: { width: row.width, height: row.height },
      deviceScaleFactor: row.dsf,
      hasTouch: row.touch,
      contextOptions: { reducedMotion: "reduce" },
    });

    test(isReportOnly(row) ? "los 10 módulos (informe)" : "los 10 módulos no se salen de lado", async ({ page }) => {
      await forceFlags(page, REFRESH);
      await signIn(page);
      await expect(page.locator("html")).toHaveAttribute("data-look", "refresh");
      const offenders: string[] = [];
      for (const path of MODULES) {
        const overflow = await overflowOf(page, path);
        if (overflow > 1) offenders.push(`${path} +${overflow}px`);
      }
      if (isReportOnly(row)) {
        for (const o of offenders) {
          test.info().annotations.push({ type: "report", description: `${rowName(row)}: ${o}` });
        }
        return;
      }
      expect(offenders, "modules that scroll sideways").toEqual([]);
    });

    if (row.width <= SETUP_NARROW_MAX_PX) {
      test("Configuración: la barra «Paso N de 8» queda a la vista bajo la cápsula", async ({ page }) => {
        await forceFlags(page, { ...REFRESH, module_config_setup: true });
        await signIn(page);
        await page.goto("/configuracion");
        const stepBar = page.locator(".setup-mobilebar");
        await expect(stepBar).toBeVisible();
        await expect(stepBar).toContainText(/Paso \d+ de 8|Papelera/);
        // Halfway and to the end: a sticky bar is only tested once the page moves.
        let moved = 0;
        for (const at of [0.5, 1] as const) {
          await page.evaluate((f) => window.scrollTo(0, f * document.documentElement.scrollHeight), at);
          await page.waitForTimeout(100);
          const where = await page.evaluate(() => {
            const bar = document.querySelector(".topbar")?.getBoundingClientRect();
            const step = document.querySelector(".setup-mobilebar")?.getBoundingClientRect();
            return { barBottom: bar?.bottom ?? 0, top: step?.top ?? -1, bottom: step?.bottom ?? -1, vh: window.innerHeight };
          });
          const scrolled = await page.evaluate(() => window.scrollY > 0);
          if (!scrolled) continue;
          moved += 1;
          expect(where.top, `the step bar left the window ${JSON.stringify(where)}`).toBeGreaterThanOrEqual(0);
          expect(where.bottom, `the step bar left the window ${JSON.stringify(where)}`).toBeLessThanOrEqual(where.vh);
          expect(where.top, `the capsule covers the step bar ${JSON.stringify(where)}`).toBeGreaterThanOrEqual(Math.max(0, where.barBottom) - 1);
        }
        expect(moved, "the page never scrolled, so the sticky bar went untested").toBeGreaterThan(0);
        await expect.poll(() => desbordeLateral(page)).toBeLessThanOrEqual(1);
      });
    }
  });
}
