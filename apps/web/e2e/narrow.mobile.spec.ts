import { expect, test } from "@playwright/test";

import { API_URL, apiAsOwner, openModule, signIn } from "./helpers";

/**
 * The same screens at phone width.
 *
 * The uniforms panels branch on a 768px breakpoint — the order list drops its
 * table for cards, the till stacks — and a branch nobody exercises is a branch
 * that breaks in front of an audience. This walks the two surfaces the school
 * uses standing up (the till and the order list) on a Pixel-sized viewport.
 *
 * It reads rather than writes: the from-zero walkthrough owns the writes, and
 * two suites creating pupils in the same database would make each other's
 * assertions depend on running order.
 */
test.describe("the school on a phone", () => {
  test("the dashboard fits and opens a module", async ({ page }) => {
    await signIn(page);

    // No horizontal scroll: the surest sign a desktop layout leaked through.
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    );
    expect(overflow, "the dashboard scrolls sideways on a phone").toBeLessThanOrEqual(1);

    await openModule(page, "Inventario");
    await expect(page.getByRole("heading", { name: "Inventario", level: 1 })).toBeVisible();
  });

  test("the till's four tabs are all reachable at phone width", async ({ page }) => {
    await signIn(page);
    await openModule(page, "Inventario");

    for (const name of ["Mostrador", "Pedidos", "Existencias", "Catálogo"]) {
      await page.getByRole("tab", { name }).click();
      await expect(page.getByRole("tab", { name })).toHaveAttribute("aria-selected", "true");
    }

    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    );
    expect(overflow, "the uniforms screen scrolls sideways on a phone").toBeLessThanOrEqual(1);
  });

  test("the order list answers on a phone as it does on a desktop", async ({ request }) => {
    // The narrow layout is a rendering choice, not a different query: the same
    // endpoint backs both, and this pins that it is still reachable and shaped.
    const api = await apiAsOwner(request);
    const res = await api.get(`${API_URL}/api/v1/uniforms/orders`);
    expect(res.status(), await res.text()).toBe(200);
    const body = (await res.json()) as { orders: unknown[] };
    expect(Array.isArray(body.orders)).toBe(true);
  });
});
