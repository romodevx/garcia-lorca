import { expect, test, type Locator, type Page } from "@playwright/test";

import { desbordeLateral, forceFlags, signIn } from "./helpers";
import { HOME_ROWS, isReportOnly, rowName, type Row } from "./matrix";

/**
 * The refreshed home (module_ui_refresh ON) across PLAN-redesign-home §4.3:
 * every viewport and deviceScaleFactor row, every assertion BLOCKING (D9).
 *
 * Jsdom applies no CSS, so none of this can be defended in a unit test: the
 * grid, the hover peek, the sticky capsule and the focus ring only exist once
 * a browser resolves globals.css.
 *
 * Motion is reduced so the layout is measured at rest: the areas' entrance and
 * the peek's transition would otherwise be measured mid-flight. The hover
 * peek's behaviour does not depend on its transition.
 */

const REFRESH = { module_ui_refresh: true } as const;
const CLASSIC = { module_ui_refresh: false } as const;

/** Tolerance in CSS px for sub-pixel rounding at fractional zoom (1.5×). */
const EPS = 1;
/** Contrast WCAG 2.2 · 1.4.11 asks of a focus indicator. */
const MIN_RING_CONTRAST = 3;
/** D7: the home never spreads past this, however wide the window. */
const HOME_MAX_PX = 1320;
/** The house's minimum target (ui-forms-and-modals: 44×44). */
const MIN_TARGET_PX = 44;
/** How far a focus ring reaches outside its box: 2px offset + 2px outline. */
const RING_PX = 4;
/** Enough presses to walk the whole home (two stops per card, 17 modules, KPIs, bar). */
const TAB_BUDGET = 70;

async function openHome(page: Page, flags: Readonly<Record<string, boolean>>): Promise<void> {
  await forceFlags(page, flags);
  await signIn(page);
  if (flags.module_ui_refresh === true) {
    await expect(page.locator("html")).toHaveAttribute("data-look", "refresh");
    await expect(page.locator(".home-card").first()).toBeVisible();
  }
}

/**
 * Text that is cut: wider or taller than its own box while that box hides the
 * rest, or poking out of an ancestor that clips. Returns one line per offender.
 */
async function clippedText(scope: Locator): Promise<string[]> {
  return scope.evaluateAll((els, eps) => {
    const clips = (s: CSSStyleDeclaration): boolean =>
      [s.overflowX, s.overflowY].some((o) => o !== "visible");
    const out: string[] = [];
    for (const el of els as HTMLElement[]) {
      const label = (el.textContent ?? "").trim().slice(0, 40);
      if (label === "" || el.getClientRects().length === 0) continue;
      const own = getComputedStyle(el);
      if (clips(own) && (el.scrollWidth > el.clientWidth + eps || el.scrollHeight > el.clientHeight + eps)) {
        out.push(`${label} — cut inside its own box`);
        continue;
      }
      const r = el.getBoundingClientRect();
      if (r.left < -eps || r.right > document.documentElement.clientWidth + eps) {
        out.push(`${label} — outside the window (${Math.round(r.left)}..${Math.round(r.right)})`);
        continue;
      }
      for (let a = el.parentElement; a !== null && a !== document.body; a = a.parentElement) {
        if (!clips(getComputedStyle(a))) continue;
        const ar = a.getBoundingClientRect();
        if (r.top < ar.top - eps || r.bottom > ar.bottom + eps || r.left < ar.left - eps || r.right > ar.right + eps) {
          out.push(`${label} — cut by .${a.className.toString().split(" ")[0]}`);
          break;
        }
      }
    }
    return out;
  }, EPS);
}

/**
 * Words broken in two: `overflow-wrap: anywhere` never clips, it splits
 * ("Calificacion / es"), so a word wider than its box is a cut too.
 */
async function splitWords(scope: Locator): Promise<string[]> {
  return scope.evaluateAll((els) => {
    const out: string[] = [];
    for (const el of els as HTMLElement[]) {
      const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
      for (let node = walker.nextNode(); node !== null; node = walker.nextNode()) {
        const text = node.textContent ?? "";
        for (const m of text.matchAll(/\S+/g)) {
          const range = document.createRange();
          range.setStart(node, m.index);
          range.setEnd(node, m.index + m[0].length);
          // One line ⇒ one rect; a word broken over two lines has two.
          const lines = new Set([...range.getClientRects()].filter((r) => r.width > 0).map((r) => Math.round(r.top)));
          if (lines.size > 1) out.push(`${m[0]} split over ${lines.size} lines`);
        }
      }
    }
    return out;
  });
}

/** The visible text of the home a person reads: titles, descriptions, area headings. */
const READABLE = ".home-greeting, .home-area__title, .home-card__main, .home-card__desc, .home-kpi__main";

/** The first card that has shortcuts (D10: not every card does). */
function cardWithShortcuts(page: Page): Locator {
  return page.locator(".home-card--peek").first();
}

/**
 * Walks the focus with Tab and back with Shift+Tab; after each press, the
 * focused thing (the whole card when its stretched button has the focus) must
 * not sit under a sticky top bar (WCAG 2.2 · 2.4.11).
 */
async function focusUnderBar(page: Page): Promise<string[]> {
  const offenders: string[] = [];
  for (const key of ["Tab", "Shift+Tab"] as const) {
    for (let i = 0; i < TAB_BUDGET; i += 1) {
      await page.keyboard.press(key);
      const hit = await page.evaluate((ring) => {
        const el = document.activeElement;
        const bar = document.querySelector(".topbar");
        if (!(el instanceof HTMLElement) || el === document.body || bar === null || bar.contains(el)) return null;
        // Next's dev overlay is a focusable custom element that production never ships.
        if (el.tagName === "NEXTJS-PORTAL") return null;
        const pos = getComputedStyle(bar).position;
        if (pos !== "sticky" && pos !== "fixed") return null;
        // A card's stretched button draws its ring around the whole card.
        const stretched = el.matches(".home-card__main, .home-kpi__main");
        const box = ((stretched ? el.closest(".home-card, .home-kpi") : null) ?? el).getBoundingClientRect();
        const barBottom = bar.getBoundingClientRect().bottom;
        // Only what is on screen counts: the browser scrolls a focused thing in.
        if (box.bottom <= 0 || box.top >= window.innerHeight) return null;
        if (box.top - ring >= barBottom) return null;
        const name = (el.getAttribute("aria-label") ?? el.textContent ?? "").trim().slice(0, 30);
        return `<${el.tagName.toLowerCase()}.${el.className.toString().split(" ")[0]}> ${name} (ring top ${Math.round(box.top - ring)} < bar ${Math.round(barBottom)})`;
      }, RING_PX);
      if (hit !== null) offenders.push(`${key}: ${hit}`);
    }
  }
  return offenders;
}

function rowSuite(row: Row): void {
  test.describe(`inicio C · ${rowName(row)}`, () => {
    test.use({
      viewport: { width: row.width, height: row.height },
      deviceScaleFactor: row.dsf,
      hasTouch: row.touch,
      contextOptions: { reducedMotion: "reduce" },
    });

    test("sin desborde lateral y sin textos cortados", async ({ page }) => {
      await openHome(page, REFRESH);
      await expect.poll(() => desbordeLateral(page), { message: "the home scrolls sideways" }).toBeLessThanOrEqual(1);
      expect(await clippedText(page.locator(READABLE))).toEqual([]);
      expect(await splitWords(page.locator(".home-card__main, .home-area__title, .home-kpi__main"))).toEqual([]);
      // The search is the palette's door: at any width it stays a real target.
      const search = await page.locator(".topbar-search-btn").boundingBox();
      expect(search?.width ?? 0, "the search trigger collapsed").toBeGreaterThanOrEqual(MIN_TARGET_PX);
    });

    test("los accesos del peek se alcanzan y no se cortan", async ({ page }) => {
      await openHome(page, REFRESH);
      const card = cardWithShortcuts(page);
      const more = card.locator(".home-card__more");
      if (row.touch) {
        // E9: proving the touch path needs the touch media query to really match.
        expect(await page.evaluate(() => matchMedia("(any-pointer: coarse)").matches)).toBe(true);
        await expect(more, "the ⋯ is always there on touch").toHaveCSS("opacity", "1");
        // Closed means closed: `hidden` keeps the shortcuts off screen and out of Tab.
        await expect(card.locator(".home-card__shortcut").first()).toBeHidden();
        await expect(page.locator(".home-card__peek[hidden] .home-card__shortcut:visible")).toHaveCount(0);
        await more.tap();
      } else {
        await card.hover();
      }
      const shortcuts = card.locator(".home-card__shortcut");
      await expect(shortcuts.first()).toBeVisible();
      for (const s of await shortcuts.all()) await expect(s).toBeVisible();
      expect(await clippedText(shortcuts)).toEqual([]);
      await expect.poll(() => desbordeLateral(page)).toBeLessThanOrEqual(1);
      if (row.touch) {
        await more.tap();
        await expect(shortcuts.first(), "a second tap on the ⋯ closes the peek").toBeHidden();
      }
      if (!row.touch) {
        // The keyboard and a hybrid's ⋯ open the same peek inside the flow.
        await more.click();
        await expect(more).toHaveAttribute("aria-expanded", "true");
        expect(await clippedText(card.locator(".home-card__shortcut"))).toEqual([]);
      }
    });

    test("el foco nunca queda bajo la barra", async ({ page }) => {
      await openHome(page, REFRESH);
      expect(await focusUnderBar(page)).toEqual([]);
    });

    if (row.width >= 2560) {
      test(`el inicio no pasa de ${HOME_MAX_PX}px`, async ({ page }) => {
        await openHome(page, REFRESH);
        const ancho = await page.locator(".home").evaluate((el) => el.getBoundingClientRect().width);
        expect(ancho).toBeLessThanOrEqual(HOME_MAX_PX);
      });
    }

    test("flag OFF: el inicio de hoy no se sale de lado", async ({ page }) => {
      await openHome(page, CLASSIC);
      await expect(page.locator("html")).not.toHaveAttribute("data-look", "refresh");
      const overflow = await desbordeLateral(page);
      if (isReportOnly(row)) {
        // OFF is today's home, outside the redesign's blocking scope (D9).
        if (overflow > 1) {
          test.info().annotations.push({ type: "report", description: `OFF home overflows ${overflow}px at ${rowName(row)}` });
        }
        return;
      }
      expect(overflow, "today's home scrolls sideways").toBeLessThanOrEqual(1);
    });
  });
}

for (const row of HOME_ROWS) rowSuite(row);

/** Relative luminance contrast between two sRGB colors (WCAG 2.x). */
function contrast(a: readonly number[], b: readonly number[]): number {
  const lum = (c: readonly number[]): number => {
    const [r, g, bl] = c.map((v) => {
      const s = v / 255;
      return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
    });
    return 0.2126 * (r ?? 0) + 0.7152 * (g ?? 0) + 0.0722 * (bl ?? 0);
  };
  const [hi, lo] = [lum(a), lum(b)].sort((x, y) => y - x);
  return ((hi ?? 0) + 0.05) / ((lo ?? 0) + 0.05);
}

/** The ring of the focused thing, and what it sits on, as sRGB triplets. */
async function ringColors(page: Page, pseudo: "::after" | null): Promise<{ style: string; ring: number[]; card: number[]; page: number[] }> {
  return page.evaluate((pseudoEl) => {
    const ctx = document.createElement("canvas").getContext("2d", { willReadFrequently: true });
    if (ctx === null) throw new Error("no 2d canvas");
    // A canvas normalizes any CSS color (hex, rgb, oklab, color-mix) to sRGB.
    const rgb = (css: string): number[] => {
      ctx.clearRect(0, 0, 1, 1);
      ctx.fillStyle = "#000";
      ctx.fillStyle = css;
      ctx.fillRect(0, 0, 1, 1);
      return [...ctx.getImageData(0, 0, 1, 1).data.slice(0, 3)];
    };
    const el = document.activeElement;
    const card = el?.closest(".home-card");
    if (!(el instanceof HTMLElement) || !(card instanceof HTMLElement)) throw new Error("focus is not in a card");
    const ring = getComputedStyle(el, pseudoEl);
    return {
      style: ring.outlineStyle,
      ring: rgb(ring.outlineColor),
      card: rgb(getComputedStyle(card).backgroundColor),
      page: rgb(getComputedStyle(document.body).backgroundColor),
    };
  }, pseudo);
}

for (const theme of ["light", "dark"] as const) {
  test.describe(`anillo de foco · ${theme}`, () => {
    test.use({ viewport: { width: 1440, height: 900 }, contextOptions: { reducedMotion: "reduce" } });

    test("contraste ≥ 3:1 contra la tarjeta y contra la página", async ({ page }) => {
      await page.addInitScript((t) => localStorage.setItem("gl-theme", t), theme);
      await openHome(page, REFRESH);
      await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
      const card = cardWithShortcuts(page);
      const main = card.locator(".home-card__main");
      const more = card.locator(".home-card__more");

      // Reached by keyboard, so :focus-visible matches as it does for a person.
      await more.focus();
      await page.keyboard.press("Shift+Tab");
      await expect(main).toBeFocused();
      // The card's ring is drawn by the stretched ::after; the ⋯ uses the look's global ring.
      for (const [what, pseudo] of [["card", "::after"], ["⋯", null]] as const) {
        if (what === "⋯") {
          await page.keyboard.press("Tab");
          await expect(more).toBeFocused();
        }
        const c = await ringColors(page, pseudo);
        expect(c.style, `${what} ring style`).toBe("solid");
        expect(contrast(c.ring, c.card), `${what} ring vs card ${JSON.stringify(c)}`).toBeGreaterThanOrEqual(MIN_RING_CONTRAST);
        expect(contrast(c.ring, c.page), `${what} ring vs page ${JSON.stringify(c)}`).toBeGreaterThanOrEqual(MIN_RING_CONTRAST);
      }
    });
  });
}
