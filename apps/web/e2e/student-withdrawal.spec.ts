import { expect, test } from "@playwright/test";
import { openModule, signIn } from "./helpers";

test("director withdraws an enrolled pupil from the sheet", async ({ page }) => {
  await signIn(page);
  await openModule(page, "Estudiantes");
  await page.getByRole("button", { name: /^Trámite/ }).click();
  const sheet = page.getByRole("dialog", { name: "Ficha del alumno" });
  const rows = page.getByRole("button", { name: /Ver ficha de/ });
  await expect(rows.first()).toBeVisible();
  // The first pupil in the tab may have no active enrollment (only those can
  // be withdrawn): open sheets until one offers the action.
  let pupil = "";
  for (let i = 0; i < (await rows.count()) && !pupil; i++) {
    await rows.nth(i).click();
    const action = sheet.getByRole("button", { name: "Dar de baja" });
    if (await action.isVisible({ timeout: 3000 }).catch(() => false)) {
      pupil = ((await rows.nth(i).getAttribute("aria-label")) ?? "").replace("Ver ficha de ", "");
      await action.click();
    } else {
      await page.keyboard.press("Escape");
      await expect(sheet).not.toBeVisible();
    }
  }
  expect(pupil, "a pupil with an active enrollment").not.toBe("");
  await expect(page.getByRole("dialog")).toHaveCount(1);
  await expect(sheet.getByText(/Se cancelarán .* cargos por/)).toBeVisible();
  await sheet.getByLabel("Motivo").fill("Cambio de escuela");
  await sheet.getByRole("button", { name: "Dar de baja", exact: true }).click();
  await expect(sheet.getByText(/Baja el/)).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(sheet).not.toBeVisible();
  await page.getByRole("button", { name: /^Bajas/ }).click();
  await expect(page.getByRole("button", { name: `Ver ficha de ${pupil}` })).toBeVisible();
});
