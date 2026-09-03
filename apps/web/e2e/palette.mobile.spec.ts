import { expect, test } from "@playwright/test";

import { signIn } from "./helpers";

/**
 * The palette's front door, on a phone.
 *
 * `narrow.mobile.spec.ts` asks whether the page overflows sideways, and that is
 * a real question — but it is the only one it asks, so it stayed green while
 * the search trigger collapsed to a 34px sliver with a zero-width placeholder.
 * "It fits" and "it is usable" are different claims, and only the first one had
 * a test.
 *
 * So this file measures the trigger the way a thumb meets it.
 */

/** Apple's and Google's floor for a touch target, and the one that matters. */
const MIN_TOUCH_TARGET = 44;

test.describe("the palette can be reached from a phone", () => {
  test("the trigger is big enough to hit and says what it does", async ({ page }) => {
    await signIn(page);

    const trigger = page.locator(".topbar-search button");
    await expect(trigger).toBeVisible();

    const box = await trigger.boundingBox();
    expect(box).not.toBeNull();
    if (box !== null) {
      expect(box.width, "the search trigger is too small to hit with a thumb").toBeGreaterThanOrEqual(
        MIN_TOUCH_TARGET,
      );
      expect(box.height, "the search trigger is too short to hit with a thumb").toBeGreaterThanOrEqual(
        MIN_TOUCH_TARGET,
      );
    }

    // The visible placeholder does not fit a 412px bar (see the note in
    // globals.css), so the trigger is an icon here — but it must still SAY what
    // it opens. The accessible name is what a screen reader announces and what
    // survives the label being hidden; asserting it is how "the icon is enough"
    // stays a decision rather than becoming an omission.
    await expect(trigger).toHaveAccessibleName(/buscar/i);
  });

  test("opens the palette and fills the screen properly", async ({ page }) => {
    await signIn(page);
    await page.locator(".topbar-search button").click();

    const dialog = page.locator("[cmdk-dialog]");
    await expect(dialog).toBeVisible();

    const box = await dialog.boundingBox();
    const viewport = page.viewportSize();
    expect(box).not.toBeNull();
    expect(viewport).not.toBeNull();
    if (box !== null && viewport !== null) {
      // Not a bottom sheet: the same dialog, given nearly the whole width.
      expect(box.width).toBeGreaterThan(viewport.width * 0.9);
      expect(box.y + box.height, "the palette runs off the bottom on a phone").toBeLessThanOrEqual(
        viewport.height,
      );
    }
  });
});
