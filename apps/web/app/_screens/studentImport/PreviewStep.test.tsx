import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { StudentImportResult } from "../../studentImport/api";
import { PreviewStep } from "./PreviewStep";
import { ALL_ALREADY, ALREADY_TO_MOVE, LEVELS, MATCHED, UNRESOLVED, importRow } from "./fixtures";

function renderPreview(result: StudentImportResult, over: Partial<Parameters<typeof PreviewStep>[0]> = {}) {
  const props = {
    result,
    levels: LEVELS,
    onChoose: vi.fn(),
    onCapacity: vi.fn(),
    onImport: vi.fn(),
    onBack: vi.fn(),
    onCancel: vi.fn(),
    onOpenTuition: vi.fn(),
    focusUnresolved: 0,
    ...over,
  };
  render(<PreviewStep {...props} />);
  return { ...props, user: userEvent.setup() };
}

describe("the preview's frame", () => {
  it("says nothing has been saved yet", () => {
    renderPreview(MATCHED);
    expect(screen.getByText("Vista previa: todavía no se ha guardado nada.")).toBeInTheDocument();
  });

  it("shows counts that add up to the file, with warnings as a subset of the new ones", () => {
    renderPreview(MATCHED);
    const cards = screen.getByRole("group", { name: "Resumen del archivo" });
    expect(within(cards).getByText("Nuevos").parentElement).toHaveTextContent("Nuevos2se importan · 1 con avisos");
    expect(within(cards).getByText("Ya estaban").parentElement).toHaveTextContent("1");
    expect(within(cards).getByText("No se importan").parentElement).toHaveTextContent("1");
    expect(within(cards).queryByText("Pendientes")).not.toBeInTheDocument();
    expect(screen.getByText("4 filas en el archivo")).toBeInTheDocument();
  });

  it("states the O5 terms above a button that names how many and where", () => {
    renderPreview(MATCHED);
    expect(screen.getByText(/Se inscribirán en el ciclo 2026–2027 desde su primer día, sin cargo de inscripción/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Importar 2 alumnos a Primaria" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Atrás" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cancelar" })).toBeInTheDocument();
  });

  it("imports when everything is resolved", async () => {
    const { user, onImport } = renderPreview(MATCHED);
    await user.click(screen.getByRole("button", { name: "Importar 2 alumnos a Primaria" }));
    expect(onImport).toHaveBeenCalledOnce();
  });

  it("offers no import when every pupil was already there", () => {
    renderPreview(ALL_ALREADY);
    expect(screen.getByText("Estos 2 alumnos ya estaban registrados (misma CURP). No se duplicó nada.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Importar/ })).not.toBeInTheDocument();
  });

  it("offers to move earlier imports to the cycle start even when nobody is new (T14)", async () => {
    const { user, onImport } = renderPreview(ALREADY_TO_MOVE);
    expect(
      screen.getByText("2 inscripciones de una importación anterior se moverán al inicio del ciclo: esos alumnos se cobrarán desde el primer mes, como a todos."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Importar/ })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Mover 2 inscripciones al inicio del ciclo" }));
    expect(onImport).toHaveBeenCalledOnce();
  });

  it("says nothing about moves when there are none", () => {
    renderPreview(MATCHED);
    expect(screen.queryByText(/se moverá/)).not.toBeInTheDocument();
  });
});

describe("level mapping", () => {
  it("shows a matched level's destination with a way to change it", async () => {
    const { user, onChoose } = renderPreview(MATCHED);
    const card = screen.getByRole("region", { name: "«Primaria» del archivo" });
    expect(card).toHaveTextContent("Van a Primaria");
    await user.click(within(card).getByRole("button", { name: "Cambiar" }));
    await user.click(within(card).getByRole("button", { name: "Bachillerato" }));
    expect(onChoose).toHaveBeenCalledWith("Primaria", { levelId: "l-bach" });
  });

  it("gives an unresolved level no default: the school's levels as chips, or a new one", async () => {
    const { user, onChoose } = renderPreview(UNRESOLVED);
    const card = screen.getByRole("region", { name: "Elige a dónde van los alumnos de «Preparatoria»" });
    expect(within(card).getAllByRole("button").map((b) => b.textContent)).toEqual(["Primaria", "Bachillerato", "+ Crear nivel nuevo «Preparatoria»"]);
    await user.click(within(card).getByRole("button", { name: "+ Crear nivel nuevo «Preparatoria»" }));
    expect(onChoose).toHaveBeenCalledWith("Preparatoria", { create: true });
  });

  it("never disables Importar: with a level unresolved it sends focus to that card instead", async () => {
    const { user, onImport } = renderPreview(UNRESOLVED);
    const button = screen.getByRole("button", { name: "Importar 2 alumnos a Preparatoria" });
    expect(button).toBeEnabled();
    await user.click(button);
    expect(onImport).not.toHaveBeenCalled();
    expect(screen.getByRole("region", { name: "Elige a dónde van los alumnos de «Preparatoria»" })).toHaveFocus();
  });

  it("warns per level when there is no tuition table, with a way to Colegiaturas", async () => {
    const noTuition: StudentImportResult = { ...MATCHED, levels: [{ fileName: "Primaria", status: "matched", levelId: "l-pri", levelName: "Primaria", students: 4, noTuition: true }] };
    const { user, onOpenTuition } = renderPreview(noTuition);
    expect(screen.getByText("Sin tabulador en el ciclo activo: no se cobrará colegiatura hasta que lo captures")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Ir a Colegiaturas" }));
    expect(onOpenTuition).toHaveBeenCalled();
  });
});

describe("what gets created", () => {
  it("summarizes the new catalog collapsed, and edits a new group's capacity on blur", async () => {
    const { user, onCapacity } = renderPreview(MATCHED);
    const summary = screen.getByText("Se crearán 0 niveles, 0 grados, 1 grupo");
    await user.click(summary);
    const cap = screen.getByLabelText("Cupo de Primaria 1° A");
    expect(cap).toHaveValue(30);
    await user.clear(cap);
    await user.type(cap, "35");
    expect(onCapacity).not.toHaveBeenCalled();
    await user.tab();
    expect(onCapacity).toHaveBeenCalledWith("Primaria|01|A", 35);
  });

  it("refuses a capacity under 1 on blur and keeps the preview as it was", async () => {
    const { user, onCapacity } = renderPreview(MATCHED);
    await user.click(screen.getByText("Se crearán 0 niveles, 0 grados, 1 grupo"));
    const cap = screen.getByLabelText("Cupo de Primaria 1° A");
    await user.clear(cap);
    await user.type(cap, "0");
    await user.tab();
    expect(onCapacity).not.toHaveBeenCalled();
    expect(screen.getByText("El cupo es un número entero desde 1.")).toBeInTheDocument();
  });
});

describe("the table", () => {
  it("opens on «No se importan» and words the reason in Spanish", () => {
    renderPreview(MATCHED);
    expect(screen.getByRole("tab", { name: /No se importan/ })).toHaveAttribute("aria-selected", "true");
    const table = screen.getByRole("table");
    expect(within(table).getAllByRole("columnheader").map((h) => h.textContent)).toEqual(["# fila", "Alumno", "CURP", "Destino", "Estado", "Motivo"]);
    expect(within(table).getByText("Baja en el sistema anterior")).toBeInTheDocument();
    expect(within(table).getByText("No se importa")).toBeInTheDocument();
  });

  it("shows the first reason and a «+N» that expands the rest", async () => {
    const { user } = renderPreview(MATCHED);
    await user.click(screen.getByRole("tab", { name: /Con avisos/ }));
    const more = screen.getByRole("button", { name: "+1" });
    expect(screen.queryByText("Correo inválido")).not.toBeInTheDocument();
    await user.click(more);
    expect(screen.getByText("Correo inválido")).toBeInTheDocument();
  });

  it("searches by name or CURP", async () => {
    const { user } = renderPreview(MATCHED);
    await user.click(screen.getByRole("tab", { name: /Todos/ }));
    await user.type(screen.getByLabelText("Buscar por nombre o CURP"), "ruxb");
    expect(within(screen.getByRole("table")).getAllByRole("row")).toHaveLength(2);
  });

  it("pages 50 at a time", async () => {
    const rows = Array.from({ length: 60 }, (_, i) => importRow({ row: i + 2, name: `ALUMNO ${i}` }));
    const { user } = renderPreview({ ...MATCHED, rows, summary: { ...MATCHED.summary, total: 60, new: 60, warnings: 0, already: 0, skipped: 0 } });
    expect(within(screen.getByRole("table")).getAllByRole("row")).toHaveLength(51);
    expect(screen.getByText("Mostrando 1–50 de 60")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Siguiente" }));
    expect(screen.getByText("Mostrando 51–60 de 60")).toBeInTheDocument();
  });
});
