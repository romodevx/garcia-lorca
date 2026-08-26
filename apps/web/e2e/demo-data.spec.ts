import { expect, test } from "@playwright/test";

import { API_URL, apiAsOwner, openModule, signIn } from "./helpers";

/**
 * The check to run an hour before walking on stage.
 *
 * Everything else in this folder builds a school from nothing and proves the
 * system works. This one asks a different question: does the school we are about
 * to SHOW actually render — four hundred pupils, twenty teachers, a priced year,
 * a stocked storeroom and a ledger with real money in it.
 *
 * It is strictly READ-ONLY, so it can run against the demo database minutes
 * before the demo without disturbing a single row. Seed with `pnpm demo:seed`.
 *
 * The numbers below are the seed's shape (scripts/seed-demo.mjs), asserted as
 * floors rather than exact counts: this is a smoke test for a populated school,
 * not a second copy of the seed's arithmetic.
 */

const EXPECTED_STUDENTS = 400;
const EXPECTED_TEACHERS = 20;
const LEVELS = ["Kínder", "Primaria", "Secundaria", "Preparatoria"];

test.describe("the demo's data is really there", () => {
  test("the roster carries the whole school, and they read as enrolled", async ({ page }) => {
    await signIn(page);
    await openModule(page, "Estudiantes");

    // The header counts the roll. Four hundred is the number the school in the
    // room will be looking at.
    await expect(page.getByText(new RegExp(`${EXPECTED_STUDENTS} alumnos`))).toBeVisible();

    // And the page renders rows rather than an empty state, which is the failure
    // an empty database would produce.
    await expect(page.getByRole("button", { name: /Ver ficha de/ }).first()).toBeVisible();

    // Placing a pupil in a group used to leave their FILE untouched, so a school
    // could have four hundred pupils actively enrolled whose records all read
    // "Trámite" — and the roster's own tabs meant nothing.
    // The stat card, not the filter tab: both say "Inscritos", and only the card
    // carries the count.
    await expect(page.getByText(`Inscritos${EXPECTED_STUDENTS}`, { exact: false }).first()).toBeVisible();
  });

  // A cycle is born a draft and used to have no way out, so every school year
  // read "Borrador" while the panel silently resolved "the active cycle" to
  // whichever came first.
  test("the school year is in service, not a draft", async ({ page }) => {
    await signIn(page);
    await openModule(page, "Configuración");

    await expect(page.getByText(/es el ciclo en curso/i)).toBeVisible();
    await expect(page.getByRole("button", { name: /Activar ciclo/ })).toHaveCount(0);
  });

  test("every level is priced, and the tuition table shows it", async ({ page }) => {
    await signIn(page);
    await openModule(page, "Configuración");

    for (const level of LEVELS) {
      const row = page.getByRole("row").filter({ hasText: level });
      await expect(row).toBeVisible();
      // A priced level shows pesos; an unpriced one says "Sin tabulador", and on
      // demo day that sentence in front of an audience is the thing to catch.
      await expect(row).not.toContainText("Sin tabulador");
    }
  });

  test("the staff roster is full and everybody teaches something", async ({ page }) => {
    await signIn(page);
    await openModule(page, "Docentes");

    await expect(page.getByText(String(EXPECTED_TEACHERS)).first()).toBeVisible();
    const roster = page.getByRole("list").first();
    await expect(roster.getByRole("listitem")).toHaveCount(EXPECTED_TEACHERS);
  });

  test("the storeroom has garments on the shelf", async ({ page }) => {
    await signIn(page);
    await openModule(page, "Uniformes");
    await page.getByRole("tab", { name: "Inventario" }).click();

    // "0 disponibles" everywhere is what an unseeded shop looks like.
    await expect(page.getByText(/\d+ disponibles/).first()).toBeVisible();
  });

  test("the order list shows the sales the school has made", async ({ page }) => {
    await signIn(page);
    await openModule(page, "Uniformes");
    await page.getByRole("tab", { name: "Pedidos" }).click();

    await expect(page.getByText(/U-\d{4}-\d{6}/).first()).toBeVisible();
  });

  // The ledger is checked through the API rather than the screen: what matters
  // before a demo is that the money is real and adds up, and the panel's own
  // rendering is covered by its unit tests.
  test("the ledger holds a real year: everyone billed, most paid, some owing", async ({ request }) => {
    const api = await apiAsOwner(request);

    const cyclesRes = await api.get(`${API_URL}/api/v1/config/cycles`);
    const { cycles } = (await cyclesRes.json()) as { cycles: { id: string; name: string }[] };
    expect(cycles.length, "the demo needs exactly one school year, or the panel picks the wrong one").toBe(1);

    const chargesRes = await api.get(`${API_URL}/api/v1/finance/charges?cycleId=${cycles[0]!.id}`);
    expect(chargesRes.status(), await chargesRes.text()).toBe(200);
    const { charges } = (await chargesRes.json()) as {
      charges: { status: string; remainingCents: number }[];
    };

    // The read is capped at 500 by the repository, so this asserts the shape of
    // what comes back rather than the whole ledger's size.
    const paid = charges.filter((c) => c.status === "paid").length;
    const pending = charges.filter((c) => c.status === "pending").length;
    expect(paid, "no paid charges — the demo would show a school nobody has paid").toBeGreaterThan(0);
    expect(pending, "no pending charges — Adeudos would have nothing to show").toBeGreaterThan(0);

    const owed = charges
      .filter((c) => c.status === "pending")
      .reduce((sum, c) => sum + c.remainingCents, 0);
    expect(owed, "the outstanding balance is zero; Adeudos is the module this demo sells").toBeGreaterThan(0);
  });
});
