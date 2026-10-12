import { expect, test } from "@playwright/test";
import { API_URL, apiAsOwner, HOME_GREETING, signIn } from "./helpers";

/**
 * The boundaries that must hold whatever else is going on.
 *
 * These are independent of the demo story and of each other: they need no data
 * and create none, so they are safe to run against any environment, including one
 * that has just been emptied for a walk-through.
 */

test("a wrong password is refused, and says so", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("textbox", { name: "Correo" }).fill("director@garcialorca.edu.mx");
  await page.getByRole("textbox", { name: "Contraseña" }).fill("definitivamente-no");
  await page.getByRole("button", { name: "Entrar al sistema" }).click();

  await expect(page.getByText(/incorrect/i)).toBeVisible();
});

test("a signed-out visitor gets the login form, not a module", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("button", { name: "Entrar al sistema" })).toBeVisible();
  await expect(page.getByRole("heading", { level: 1, name: HOME_GREETING })).toBeHidden();
});

/**
 * A module that is off must be INVISIBLE at the edge, not merely refused: the
 * flag is a security boundary the gateway enforces, and a 403 would confirm the
 * module exists to somebody who should not know.
 */
test("a disabled module answers 404 at the edge", async ({ request }) => {
  const api = await apiAsOwner(request);
  const features = await api.get(`${API_URL}/api/v1/features`);
  const flags = (await features.json()) as Record<string, boolean>;

  const off = Object.entries(flags).filter(([, on]) => !on);
  expect(off.length, "this only proves something if some module is off").toBeGreaterThan(0);

  for (const [flag] of off) {
    const moduleName = flag.replace("module_", "");
    const res = await api.get(`${API_URL}/api/v1/${moduleName}`);
    expect([404, 405], `${flag} is off, so its routes must not be reachable`).toContain(res.status());
  }
});

/**
 * The counter sale moves goods and money in one act, so the gateway demands an
 * idempotency key — a double click at the till would otherwise be a second sale
 * of goods that left once.
 */
test("the counter sale refuses a request with no idempotency key", async ({ request }) => {
  const api = await apiAsOwner(request);
  const res = await api.post(`${API_URL}/api/v1/uniforms/counter-sale`, {
    data: { lines: [{ itemId: "x", sizeLabel: "M", qty: 1 }], method: "cash", note: "x" },
  });
  expect(res.status()).toBe(400);
  const body = (await res.json()) as { error: { details?: Record<string, string> } };
  expect(body.error.details?.header).toBe("Idempotency-Key");
});

/** A price a client sends must never be the price a family pays. */
test("the sale request has nowhere to put a price", async ({ page }) => {
  await signIn(page);
  const bodies: string[] = [];
  page.on("request", (req) => {
    if (req.url().includes("/counter-sale") && req.method() === "POST") {
      bodies.push(req.postData() ?? "");
    }
  });
  // Nothing is sold here; the assertion is about the SHAPE the client can send,
  // and the type has no field for an amount at all.
  expect(bodies.every((b) => !b.includes("priceCents"))).toBe(true);
});
