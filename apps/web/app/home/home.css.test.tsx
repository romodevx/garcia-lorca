import { readFileSync } from "node:fs";
import { screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { USERS } from "../_design/data";
import { DESKTOP, clearHovered, declaredStyle, markHovered, parseStylesheet } from "../_design/testing/declaredStyle";
import { HomeView } from "./HomeView";
import { renderHome } from "./testing/renderHome";

vi.mock("../students/api", () => ({ listStudents: vi.fn().mockResolvedValue([]) }));
vi.mock("../config/api", () => ({ listCycles: vi.fn().mockResolvedValue([]) }));
vi.mock("../finance/api", () => ({ listCharges: vi.fn().mockResolvedValue([]) }));

/*
 * The refreshed home's layout and motion contract (PLAN-redesign-home §3.2,
 * §4.2), read from globals.css. Rendered nodes go through the declared
 * cascade; pointer and motion media (which no jsdom session matches) are
 * read as rules. The real browsers check it in F4's matrix.
 */

const NOW = new Date("2026-06-12T09:00:00");
const PERMS = ["students:read", "students:create", "finance:read", "finance:write", "config:read", "attendance:read", "grades:read", "users:read", "comms:read"];
const RULES = parseStylesheet(readFileSync("app/globals.css", "utf8"));

/** The declarations of the rule for exactly `selector` under media containing `media` (null: no media). */
function rule(selector: string, media: string | null = null): Map<string, string> {
  const found = RULES.filter((r) => r.selectors.includes(selector) && (media === null ? r.media === null : (r.media ?? "").includes(media)));
  if (found.length === 0) throw new Error(`no rule for ${selector} @ ${media ?? "(none)"}`);
  return new Map(found.flatMap((r) => r.declarations.map((d) => [d.prop, d.value] as const)));
}

const style = (el: Element): Map<string, string> => declaredStyle(el, DESKTOP);

function home(): void {
  renderHome(<HomeView user={USERS.admin} onOpen={() => {}} permissions={PERMS} now={NOW} />, true, { module_students: true });
}

afterEach(() => clearHovered());

describe("refreshed home — layout (§4.2)", () => {
  it("caps at 1320px, centered, at every width (D7)", () => {
    home();
    const root = document.querySelector(".home");
    if (root === null) throw new Error("no .home");
    expect(style(root).get("max-inline-size")).toBe("1320px");
    expect(style(root).get("margin-inline")).toBe("auto");
  });

  it("lays every area out on an auto-fit grid that reflows down to 320px", () => {
    home();
    for (const grid of document.querySelectorAll(".home-grid, .home-kpis")) {
      expect(style(grid).get("grid-template-columns")).toBe("repeat(auto-fit, minmax(min(100%, 232px), 1fr))");
    }
  });

  it("reserves the card's height in rem, so it scales with zoom", () => {
    home();
    const card = screen.getByRole("button", { name: "Estudiantes" }).closest("li");
    if (card === null) throw new Error("no card");
    expect(style(card).get("min-height")).toMatch(/rem$/);
    const body = card.querySelector(".home-card__body");
    expect(body && style(body).get("min-height")).toBe("calc(var(--peek-rows) * 2.75rem)");
  });

  it("gives the ⋯ a 44×44 target", () => {
    home();
    const more = screen.getByRole("button", { name: "Accesos de Estudiantes" });
    expect([style(more).get("width"), style(more).get("height")]).toEqual(["44px", "44px"]);
  });
});

describe("refreshed home — area colors (only this view, only with the flag)", () => {
  it.each([
    ["people", "var(--forest)"],
    ["academic", "var(--purple)"],
    ["money", "var(--orange)"],
    ["system", "var(--text-muted)"],
  ])("%s cards take %s", (group, color) => {
    expect(rule(`.home-card[data-group="${group}"]`).get("--area-c")).toBe(color);
  });

  it("tints the icon square and pulls the glyph toward the text (F2b's ≥3:1 rule)", () => {
    const icon = rule(".home-card__icon");
    expect(icon.get("background")).toBe("color-mix(in oklab, var(--area-c) var(--icon-tint, 13%), var(--surface))");
    expect(icon.get("color")).toBe("color-mix(in oklab, var(--area-c) var(--icon-ink-share, 60%), var(--text))");
  });
});

describe("refreshed home — motion (§3.2, D15)", () => {
  it("one entrance per section, 60ms apart, none per card", () => {
    expect(rule(".home-area").get("animation-delay")).toBe("calc(var(--area-i, 0) * 60ms)");
    expect(rule(".home-card").get("animation")).toBeUndefined();
  });

  it("a hovered card lifts 2px", () => {
    home();
    const card = screen.getByRole("button", { name: "Estudiantes" }).closest("li");
    if (card === null) throw new Error("no card");
    markHovered(card);
    expect(style(card).get("transform")).toBe("translateY(-2px)");
  });

  it("with a mouse, the peek fades over the description after 120ms and leaves after 80ms", () => {
    const fine = "(hover: hover) and (pointer: fine)";
    const rest = rule(".home-card__peek[hidden]", fine);
    expect(rest.get("position")).toBe("absolute");
    expect(rest.get("visibility")).toBe("hidden");
    expect(rest.get("transition")).toContain("opacity .2s var(--ease) 80ms");
    const shown = rule(".home-card:hover .home-card__peek[hidden]", fine);
    expect(shown.get("visibility")).toBe("visible");
    expect(shown.get("transition")).toContain("opacity .2s var(--ease) 120ms");
  });

  it("with a mouse the ⋯ waits for hover or focus; with touch it is always there", () => {
    expect(rule(".home-card:not(:hover):not(:focus-within) .home-card__more", "(hover: hover) and (pointer: fine)").get("opacity")).toBe("0");
    expect(rule(".home-card:not(:hover):not(:focus-within) .home-card__more", "(any-pointer: coarse)").get("opacity")).toBe("1");
  });

  it("reduced motion: no entrance, no lift, no peek transition", () => {
    const reduce = "(prefers-reduced-motion: reduce)";
    expect(rule(".home-area", reduce).get("animation")).toBe("none");
    expect(rule(".home-card:hover", reduce).get("transform")).toBe("none");
    expect(rule(".home-card:hover .home-card__peek[hidden]", reduce).get("transition")).toBe("none");
  });
});
