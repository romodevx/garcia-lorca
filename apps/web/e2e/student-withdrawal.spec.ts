import { expect, test } from "@playwright/test";
import { openModule, signIn } from "./helpers";

test("director withdraws an enrolled pupil from the sheet", async ({ page }) => {
  await signIn(page);
  await openModule(page, "Estudiantes");
  await page.getByRole("button", { name: /^Inscritos/ }).click();
  const openSheet = page.getByRole("button", { name: /Ver ficha de/ }).first();
  await expect(openSheet).toBeVisible();
  const pupil = ((await openSheet.getAttribute("aria-label")) ?? "").replace("Ver ficha de ", "");
  await openSheet.click();
  const sheet = page.getByRole("dialog", { name: "Ficha del alumno" });
  await sheet.getByRole("button", { name: "Dar de baja" }).click();
  const withdrawal = page.getByRole("dialog", { name: "Dar de baja" });
  await expect(withdrawal.getByText(/Se cancelarán .* cargos por/)).toBeVisible();
  await withdrawal.getByLabel("Motivo").fill("Cambio de escuela");
  await withdrawal.getByRole("button", { name: "Dar de baja" }).click();
  await expect(withdrawal).not.toBeVisible();
  await expect(sheet.getByText(/Baja el/)).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(sheet).not.toBeVisible();
  await page.getByRole("button", { name: /^Bajas/ }).click();
  await expect(page.getByRole("button", { name: `Ver ficha de ${pupil}` })).toBeVisible();
});
