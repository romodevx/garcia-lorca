import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { DrawerShell } from "./DrawerShell";

// Regression: ISSUE-001 — at 360 px the close-period footer pushed
// "Cerrar de todos modos" past the dialog's right edge: the footer row never
// wrapped, so a long primary label was clipped instead of dropping a line.
// Found by /qa on 2026-09-27
// Report: .gstack/qa-reports/qa-report-localhost-3002-2026-09-27-t7.md
describe("el pie del cajón", () => {
  it("parte el renglón cuando la acción no cabe junto a Cancelar", () => {
    render(
      <DrawerShell title="Cerrar periodo" icon="lock" onClose={() => undefined}
        footer={<><button type="button">Cancelar</button><button type="button">Cerrar de todos modos</button></>}>
        <p>Contenido</p>
      </DrawerShell>,
    );

    const footer = screen.getByRole("button", { name: "Cerrar de todos modos" }).parentElement;
    expect(footer).toHaveStyle({ flexWrap: "wrap" });
  });
});
