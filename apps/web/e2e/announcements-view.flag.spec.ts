import { expect, test, type APIRequestContext, type Page } from "@playwright/test";
import { API_URL, anyLevelId, apiAsOwner, forceFlags, openModule, signIn, uniqueSuffix } from "./helpers";

/**
 * Avisos with module_announcements_view ON ("Destinatarios al lado") and OFF
 * (today's screen), each sending ONE announcement to ONE pupil the spec makes
 * for itself: a whole-school send would fan out through comms to every family
 * in the local database.
 *
 * Its OWN Playwright project (`announcements-view`, `pnpm e2e:announcements-view`):
 * the flag is presentation-only, so it is forced in the browser by rewriting
 * `/api/v1/features`, and the gateway's answer for everything else is real.
 *
 * globalSetup empties the local database for every project. Against a school
 * you want to keep, run it with E2E_SKIP_RESET=1.
 */

const VIEW_FLAG = "module_announcements_view";
const TABLET_PORTRAIT = { width: 768, height: 1024 };

/** A CURP with the shape students validates (not a real one). */
function shapedCurp(): string {
  const consonants = "BCDFGHJKLMNPQRSTVWXYZ";
  const pick = (from: string): string => from[Math.floor(Math.random() * from.length)] ?? "B";
  return `${pick(consonants)}A${pick(consonants)}${pick(consonants)}200315MDF` +
    `${pick(consonants)}${pick(consonants)}${pick(consonants)}A${pick("0123456789")}`;
}

/** Announcing needs a cycle (D1); the first one listed, or a fresh one. */
async function ensureCycle(api: APIRequestContext): Promise<void> {
  const listed = await api.get(`${API_URL}/api/v1/config/cycles`);
  expect(listed.ok(), `list cycles: ${listed.status()}`).toBeTruthy();
  const { cycles } = (await listed.json()) as { cycles?: unknown[] };
  if ((cycles?.length ?? 0) > 0) return;
  const created = await api.post(`${API_URL}/api/v1/config/cycles`, {
    data: { name: `Avisos ${uniqueSuffix()}`, startsOn: "2026-08-01", endsOn: "2027-06-30" },
  });
  expect(created.ok(), `create cycle: ${await created.text()}`).toBeTruthy();
}

/** One pupil of the spec's own, found by a surname nobody else has. */
async function arrangePupil(api: APIRequestContext): Promise<string> {
  await ensureCycle(api);
  const surname = `Avisos${uniqueSuffix()}`;
  const res = await api.post(`${API_URL}/api/v1/students`, {
    data: { curp: shapedCurp(), firstName: "Ana", lastName: surname, levelId: await anyLevelId(api), gradeGroup: "A", guardianName: "Tutor de prueba" },
  });
  expect(res.ok(), `create pupil: ${await res.text()}`).toBeTruthy();
  return surname;
}

async function openAvisos(page: Page, viewOn: boolean): Promise<void> {
  await forceFlags(page, { [VIEW_FLAG]: viewOn });
  await signIn(page);
  await openModule(page, "Avisos");
}

async function writeAndSend(page: Page): Promise<void> {
  await page.getByLabel("Título", { exact: true }).fill("Recordatorio");
  await page.getByLabel("Mensaje", { exact: true }).fill("Traer el uniforme de deportes mañana.");
  await page.getByRole("button", { name: /Enviar aviso/ }).click();
  await expect(page.getByRole("status")).toHaveText(/^Enviado a 1 alumno/);
}

test.describe("Avisos and module_announcements_view", () => {
  test("ON: a pupil found by surname gets the announcement", async ({ page, request }) => {
    const surname = await arrangePupil(await apiAsOwner(request));
    await openAvisos(page, true);
    await expect(page.getByRole("heading", { name: "Destinatarios" })).toBeVisible();

    await page.getByRole("button", { name: "Un alumno", exact: true }).click();
    await page.getByRole("searchbox", { name: "Buscar alumno" }).fill(surname.toLowerCase());
    await page.getByRole("button", { name: new RegExp(surname, "i") }).click();
    await expect(page.getByText(/^Para /)).toContainText(new RegExp(surname, "i"));
    await writeAndSend(page);
  });

  test("OFF: today's screen still sends to one pupil", async ({ page, request }) => {
    const surname = await arrangePupil(await apiAsOwner(request));
    await openAvisos(page, false);
    await expect(page.getByRole("heading", { name: "Destinatarios" })).toHaveCount(0);

    await page.getByRole("button", { name: "Un alumno", exact: true }).click();
    await page.getByRole("button", { name: new RegExp(surname, "i") }).click();
    await writeAndSend(page);
  });

  // P5 + WCAG 2.2 · 2.4.11 (D6): one column, the footer stuck to the bottom,
  // never over the field that has the focus.
  test("ON, tablet portrait: Enviar aviso stays in view and never covers the focused field", async ({ page }) => {
    await page.setViewportSize(TABLET_PORTRAIT);
    await openAvisos(page, true);
    const message = page.getByLabel("Mensaje", { exact: true });
    await message.focus();
    const send = page.getByRole("button", { name: /Enviar aviso/ });
    await expect(send).toBeInViewport();
    const field = await message.boundingBox();
    const footer = await send.boundingBox();
    expect(field && footer && field.y + field.height <= footer.y).toBeTruthy();
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow).toBeLessThanOrEqual(0);
  });
});
