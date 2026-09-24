import { expect, type APIRequestContext, type BrowserContext, type Page } from "@playwright/test";

/**
 * Shared moves for the end-to-end suite.
 *
 * Two rules hold everything here together.
 *
 * FIXTURES ARE UNIQUE PER RUN. Every name carries a run stamp, because these
 * tests hit ONE real database that is never reset — a suite that only passes on
 * a fresh database is a suite that stops passing on the machine the demo runs on.
 *
 * SETUP GOES THROUGH THE API, ASSERTIONS GO THROUGH THE UI. Clicking through six
 * screens to arrange a precondition makes a test that fails for six reasons and
 * tells you none of them. The click-through is reserved for the thing under test.
 */

export const OWNER = {
  email: process.env.E2E_OWNER_EMAIL ?? "director@garcialorca.edu.mx",
  password: process.env.E2E_OWNER_PASSWORD ?? "Admin123.",
};

export const API_URL = process.env.E2E_API_URL ?? "http://localhost:8080";

/** A stamp unique to this run, so re-running never collides with itself. */
export function runId(): string {
  return `e2e-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 6)}`;
}

/** A short stamp for names a person will read on screen during a demo. */
export function uniqueSuffix(): string {
  return Date.now().toString(36).slice(-4);
}

/**
 * Signs in through the real form.
 *
 * It uses the UI rather than seeding a cookie because logging in is itself part
 * of the demo, and a suite that skips it would not notice the day it breaks.
 */
/**
 * Puts the director in front of the dashboard.
 *
 * The session is minted ONCE and replayed as cookies for every test after that.
 * Signing in per test is what a suite naturally does and what this one used to
 * do — and at 23 tests it drained the login throttle (burst 10, refilling one
 * every two seconds) and failed a late test for a reason that had nothing to do
 * with what that test was checking. Replaying is also closer to the truth: a
 * director signs in in the morning, not before every click.
 *
 * `forceLogin` skips the cache for the one test whose subject IS the login form.
 */
let cachedCookies: Awaited<ReturnType<BrowserContext["cookies"]>> | null = null;

export async function signIn(page: Page, forceLogin = false): Promise<void> {
  if (!forceLogin && cachedCookies !== null) {
    await page.context().addCookies(cachedCookies);
    await page.goto("/");
    await expect(page.getByRole("heading", { level: 1, name: /🌱/ })).toBeVisible({ timeout: 20_000 });
    return;
  }

  await page.goto("/");
  // ESPERAR LA REDIRECCIÓN ANTES DE ESCRIBIR. "/" monta su propio formulario y
  // en seguida navega a /entrar; el formulario de destino nace vacío, así que
  // llenar el primero pierde lo escrito y el clic manda credenciales en blanco.
  // La pantalla entonces dice "Correo o contraseña incorrectos" — un mensaje que
  // acusa a la contraseña de un fallo que fue de tiempos, y que en esta máquina
  // aparecía cada vez que el sondeo de sesión (/me 401 → refresh 401) tardaba lo
  // suficiente. Es una carrera latente de toda la suite, no de un test.
  await page.waitForURL("**/entrar", { timeout: 20_000 }).catch(() => {
    // Si nunca redirige, el formulario de "/" es el bueno: seguir adelante.
  });
  // By ROLE, not by label: "Contraseña" also matches the show-password button,
  // and a selector that resolves to two elements fails on the strictness rather
  // than on anything real.
  await page.getByRole("textbox", { name: "Correo" }).fill(OWNER.email);
  await page.getByRole("textbox", { name: "Contraseña" }).fill(OWNER.password);
  await page.getByRole("button", { name: "Entrar al sistema" }).click();

  // The greeting is the proof. Waiting for "an h1" would have passed on the
  // LOGIN page, which has one of its own — a green test that never signed in.
  await expect(page.getByRole("heading", { level: 1, name: /🌱/ })).toBeVisible({ timeout: 20_000 });

  cachedCookies = await page.context().cookies();
}

/** Opens a module from the dashboard, by the name a person reads. */
export async function openModule(page: Page, name: string): Promise<void> {
  await page.getByRole("button", { name: new RegExp(name, "i") }).first().click();
  await expect(page.getByRole("heading", { level: 1, name: new RegExp(name, "i") })).toBeVisible();
}

/**
 * An API context carrying the owner's session.
 *
 * Preconditions are arranged with this. The session lives in an HttpOnly cookie
 * the gateway sets, so the context stores it exactly as the browser would.
 */
export async function apiAsOwner(request: APIRequestContext): Promise<APIRequestContext> {
  const res = await request.post(`${API_URL}/api/v1/auth/login`, {
    data: { email: OWNER.email, password: OWNER.password },
  });
  expect(res.ok(), "the owner must be able to sign in — check the seed").toBeTruthy();
  return request;
}

/**
 * A config level to place a pupil in. A pupil's level is a reference to the
 * school's catalog (ADR 0014), so a spec that creates pupils needs one: the
 * first existing level, or a fresh one on an empty database.
 */
export async function anyLevelId(request: APIRequestContext): Promise<string> {
  const listed = await request.get(`${API_URL}/api/v1/config/levels`);
  expect(listed.ok(), `list levels: ${listed.status()}`).toBeTruthy();
  const { levels } = (await listed.json()) as { levels?: { id: string }[] };
  const first = levels?.[0];
  if (first !== undefined) return first.id;
  const created = await request.post(`${API_URL}/api/v1/config/levels`, {
    data: { name: `Primaria ${uniqueSuffix()}`, sepCode: "", enrollmentGranularity: "cycle" },
  });
  expect(created.ok(), `create level: ${await created.text()}`).toBeTruthy();
  return ((await created.json()) as { id: string }).id;
}

/** Creates a garment with a size run and stock, straight through the API. */
export async function seedGarment(
  request: APIRequestContext,
  opts: { code: string; name: string; priceCents: number; sizes: string[]; onHand: number; levelId?: string },
): Promise<{ itemId: string; stockIds: Record<string, string> }> {
  const created = await request.post(`${API_URL}/api/v1/uniforms/items`, {
    data: { code: opts.code, name: opts.name, priceCents: opts.priceCents, levelId: opts.levelId ?? "" },
  });
  expect(created.ok(), `seed garment ${opts.code}: ${created.status()}`).toBeTruthy();
  const item = (await created.json()) as { id: string };

  const sized = await request.post(`${API_URL}/api/v1/uniforms/items/${item.id}/sizes`, {
    data: { sizes: opts.sizes.map((sizeLabel, i) => ({ sizeLabel, sizeOrder: i + 1, minStock: 2 })) },
  });
  expect(sized.ok(), `seed sizes for ${opts.code}: ${sized.status()}`).toBeTruthy();
  const sizes = (await sized.json()) as { sizes: { id: string; sizeLabel: string }[] };

  const stockIds: Record<string, string> = {};
  for (const size of sizes.sizes) stockIds[size.sizeLabel] = size.id;

  if (opts.onHand > 0) {
    const received = await request.post(`${API_URL}/api/v1/uniforms/stock/receive`, {
      data: {
        lines: sizes.sizes.map((s) => ({ stockId: s.id, qty: opts.onHand })),
        note: "seed e2e",
      },
    });
    expect(received.ok(), `seed stock for ${opts.code}: ${received.status()}`).toBeTruthy();
  }
  return { itemId: item.id, stockIds };
}

/** How many pieces of one size are free right now, read straight from the API. */
export async function availableOf(
  request: APIRequestContext,
  itemId: string,
  sizeLabel: string,
): Promise<number> {
  const res = await request.get(`${API_URL}/api/v1/uniforms/stock?itemId=${itemId}`);
  expect(res.ok()).toBeTruthy();
  const body = (await res.json()) as { stock: { sizeLabel: string; available: number }[] };
  return body.stock.find((s) => s.sizeLabel === sizeLabel)?.available ?? -1;
}

/**
 * Chooses a pupil in the counter sale's picker, by name.
 *
 * It names the pupil rather than taking the first match, because the till has a
 * precondition the picker does not show: only a pupil with a live enrolment has
 * a school year to bill, so "whoever comes up first" sells to whoever happens to
 * be unenrolled. What is under test is that the picker resolves a NAME to an ID
 * at all — the field used to send the typed text where an id belongs, and the
 * sale came back a 500.
 */
export async function pickStudent(page: Page, name: string): Promise<void> {
  const search = page.getByRole("textbox", { name: "Alumno" });
  await search.fill(name);
  const match = page.getByRole("listitem").filter({ hasText: name }).first().getByRole("button");
  await expect(match, `no pupil matched ${name}`).toBeVisible();
  await match.click();
  // The picker collapses to the chosen pupil, which is the proof an id was
  // resolved rather than a string passed through.
  // exact, because the shell's theme toggle is labelled "Cambiar tema".
  await expect(page.getByRole("button", { name: "Cambiar", exact: true })).toBeVisible();
}
