import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { StudentPicker } from "./StudentPicker";

describe("StudentPicker", () => {
  it("se monta sin reventar", async () => {
    // El portal se activa en un efecto, así que hay DOS renders: uno antes de
    // montar y otro después. Cualquier hook declarado debajo del early return
    // sólo existe en el segundo, y React tira "Rendered more hooks than during
    // the previous render". Se ve al abrir Finanzas → Cobrar.
    render(
      <StudentPicker
        students={[{ id: "s1", firstName: "ANA", lastName: "RUIZ", gradeGroup: "1° A" }]}
        onPick={vi.fn()}
        onClose={vi.fn()}
      />
    );

    expect(await screen.findByRole("dialog", { name: /Buscar alumno/ })).toBeInTheDocument();
    expect(screen.getByText(/ANA/)).toBeInTheDocument();
  });
});
