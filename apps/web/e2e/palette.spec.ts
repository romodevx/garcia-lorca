import { expect, test, type Page } from "@playwright/test";

import { signIn } from "./helpers";

/**
 * The palette has to be SEEN.
 *
 * Every other test of the command palette lives in jsdom, and jsdom applies no
 * CSS. That is not a small gap: the palette shipped once with its markup
 * perfect, its matching tested, and no stylesheet at all — `cmdk` ships none of
 * its own — so `Command.Dialog` mounted into a portal with `position: static`,
 * a transparent background, and its centre 646px down an 720px-tall window.
 * A hundred and forty-one unit tests stayed green while the feature was, to the
 * person using it, absent.
 *
 * So these assertions are deliberately about GEOMETRY and PAINT rather than
 * about the DOM. "The element exists" is exactly the claim that was already
 * true when the feature was broken.
 *
 * READ-ONLY: the palette navigates and never mutates, so this belongs in the
 * `demo` project and can run minutes before a demo. Run it with
 * `pnpm demo:smoke` (which sets E2E_SKIP_RESET=1), never with a suite that
 * truncates.
 */

/** Opens the palette the way a person does, and waits for it to be mounted. */
async function openPalette(page: Page): Promise<void> {
  await page.keyboard.press("Meta+k");
  await expect(page.locator("[cmdk-input]")).toBeVisible();
}

test.describe("the command palette is visible", () => {
  test("opens over the page, on screen, and painted", async ({ page }) => {
    await signIn(page);
    await openPalette(page);

    const dialog = page.locator("[cmdk-dialog]");

    // Drawn OVER the page, not laid out inside it. `static` is what a portal
    // gets with no stylesheet, and it is what shipped.
    await expect(dialog).toHaveCSS("position", "fixed");

    // Painted. A transparent panel over a white dashboard is an invisible one
    // even when it is correctly positioned.
    const background = await dialog.evaluate((el) => getComputedStyle(el).backgroundColor);
    expect(background, "the palette needs its own surface, not the page showing through").not.toBe("rgba(0, 0, 0, 0)");

    // On screen. The shipped palette started 646px down a 720px window, so its
    // body hung below the fold: present in the DOM, absent to the user.
    const box = await dialog.boundingBox();
    expect(box, "the palette has no box at all").not.toBeNull();
    const viewport = page.viewportSize();
    expect(viewport).not.toBeNull();
    if (box !== null && viewport !== null) {
      expect(box.y, "the palette starts below the top of the window").toBeGreaterThanOrEqual(0);
      expect(
        box.y + box.height,
        "the palette runs off the bottom of the window — the school's laptops are 1366×768",
      ).toBeLessThanOrEqual(viewport.height);
    }

    // The real proof, and the one the shipped version failed outright: ask the
    // browser what is actually painted at the palette's centre. It answered
    // "nothing", because the centre was off-screen.
    const hitsItself = await dialog.evaluate((el) => {
      const r = el.getBoundingClientRect();
      const at = document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2);
      return at !== null && el.contains(at);
    });
    expect(hitsItself, "nothing is painted at the palette's centre").toBe(true);
  });

  test("shows where the keyboard is", async ({ page }) => {
    await signIn(page);
    await openPalette(page);

    // cmdk marks the active row with [data-selected], and never moves DOM focus
    // there — so the app's global :focus-visible ring cannot fire for it. If the
    // stylesheet only styles :hover, arrow keys move an invisible cursor.
    const selected = page.locator('[cmdk-item][data-selected="true"]').first();
    await expect(selected).toBeVisible();

    const [selectedBg, plainBg] = await Promise.all([
      selected.evaluate((el) => getComputedStyle(el).backgroundColor),
      page.locator('[cmdk-item]:not([data-selected="true"])').first()
        .evaluate((el) => getComputedStyle(el).backgroundColor),
    ]);
    expect(selectedBg, "the highlighted row looks identical to the others").not.toBe(plainBg);
  });

  test("teaches what to type when nothing matches", async ({ page }) => {
    await signIn(page);
    await openPalette(page);
    await page.locator("[cmdk-input]").fill("zzzzqqq");

    // "Sin resultados" alone is a dead end, and it is the moment the user is
    // most ready to be told what this box is for. The copy for that already
    // existed in both locales (`ui.hint`) and was referenced nowhere.
    await expect(page.locator("[cmdk-empty]")).toBeVisible();
    await expect(page.locator("[cmdk-empty]")).toContainText(/Escribe para navegar o crear/i);
  });

  test("closes on Escape", async ({ page }) => {
    await signIn(page);
    await openPalette(page);
    await page.keyboard.press("Escape");
    await expect(page.locator("[cmdk-input]")).toBeHidden();
  });
});
