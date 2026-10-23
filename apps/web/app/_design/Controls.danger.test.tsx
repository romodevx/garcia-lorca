import { render, screen } from "@testing-library/react";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

import { Btn } from "./Controls";

describe("Btn danger", () => {
  // --on-color-ink is near-black green in the dark theme: 3.2:1 on --red, below
  // the 4.5:1 a label needs. White on #E00000 is 4.9:1 in both themes.
  it("writes its label in --on-danger, not the theme's ink", () => {
    render(<Btn danger>Quitar acceso</Btn>);
    expect(screen.getByRole("button", { name: "Quitar acceso" }).style.color).toBe("var(--on-danger)");
  });

  it("defines --on-danger as white in both theme blocks", () => {
    const css = readFileSync(join(__dirname, "../globals.css"), "utf8");
    expect(css.match(/--on-danger:\s*#FFFFFF;/g)?.length).toBe(2);
  });
});
