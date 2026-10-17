import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as financeApi from "../../finance/api";
import { ConceptsPanel } from "./ConceptsPanel";

vi.mock("../../finance/api", () => ({
  listConcepts: vi.fn(),
  createConcept: vi.fn(),
  updateConcept: vi.fn(),
}));

const finance = vi.mocked(financeApi);

const concept = (over: Partial<financeApi.ChargeConcept>): financeApi.ChargeConcept => ({
  id: "k1", name: "COLEGIATURA", defaultAmountCents: 350000, recurrence: "monthly", active: true, kind: "custom", ...over,
});

beforeEach(() => {
  finance.listConcepts.mockResolvedValue([
    concept({}),
    concept({ id: "k2", name: "EXAMEN EXTRAORDINARIO", defaultAmountCents: 25000, recurrence: "once", active: false }),
  ]);
});
afterEach(() => vi.clearAllMocks());

describe("ConceptsPanel (CC-111)", () => {
  it("lists every concept, the archived ones marked, read with includeInactive", async () => {
    render(<ConceptsPanel canWrite={false} />);

    const table = await screen.findByRole("table", { name: "Conceptos de cobro" });
    expect(within(table).getByText("COLEGIATURA")).toBeInTheDocument();
    expect(within(table).getByText("$3,500.00")).toBeInTheDocument();
    expect(within(table).getByText("Mensual")).toBeInTheDocument();
    expect(within(table).getByText("Inactivo")).toBeInTheDocument();
    expect(finance.listConcepts).toHaveBeenCalledWith(expect.any(String), true);
  });

  it("without finance:write it offers nothing to change", async () => {
    render(<ConceptsPanel canWrite={false} />);

    await screen.findByText("COLEGIATURA");
    expect(screen.queryByRole("button", { name: /Agregar concepto/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Editar/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Desactivar/ })).not.toBeInTheDocument();
  });

  it("adds a concept inline: pesos on screen, cents on the wire", async () => {
    const user = userEvent.setup();
    finance.createConcept.mockResolvedValue(concept({ id: "k3", name: "CREDENCIAL", defaultAmountCents: 8000, recurrence: "once" }));
    render(<ConceptsPanel canWrite />);

    await user.click(await screen.findByRole("button", { name: "Agregar concepto" }));
    await user.type(screen.getByLabelText("Nombre"), "credencial");
    await user.type(screen.getByLabelText("Monto"), "80");
    await user.selectOptions(screen.getByLabelText("Se cobra"), "once");
    await user.keyboard("{Enter}");

    await waitFor(() => expect(finance.createConcept).toHaveBeenCalledWith(expect.any(String), {
      name: "CREDENCIAL", defaultAmountCents: 8000, recurrence: "once",
    }));
    expect(await screen.findByText("CREDENCIAL")).toBeInTheDocument();
  });

  it("refuses an amount it cannot read, in the cell, without calling the server", async () => {
    const user = userEvent.setup();
    render(<ConceptsPanel canWrite />);

    await user.click(await screen.findByRole("button", { name: "Agregar concepto" }));
    await user.type(screen.getByLabelText("Nombre"), "credencial");
    await user.type(screen.getByLabelText("Monto"), "ochenta");
    await user.keyboard("{Enter}");

    expect(await screen.findByRole("alert")).toHaveTextContent(/monto en pesos/i);
    expect(finance.createConcept).not.toHaveBeenCalled();
  });

  it("edits a concept in place and sends the full replace", async () => {
    const user = userEvent.setup();
    finance.updateConcept.mockResolvedValue(concept({ defaultAmountCents: 360000 }));
    render(<ConceptsPanel canWrite />);

    await user.click(await screen.findByRole("button", { name: "Editar COLEGIATURA" }));
    const amount = screen.getByLabelText("Monto de COLEGIATURA");
    await user.clear(amount);
    await user.type(amount, "3600");
    await user.click(screen.getByRole("button", { name: "Guardar COLEGIATURA" }));

    await waitFor(() => expect(finance.updateConcept).toHaveBeenCalledWith(expect.any(String), "k1", {
      name: "COLEGIATURA", defaultAmountCents: 360000, recurrence: "monthly", active: true,
    }));
    expect(await screen.findByText("$3,600.00")).toBeInTheDocument();
  });

  it("deactivates at once and keeps the way back on the row (reversible, no confirm)", async () => {
    const user = userEvent.setup();
    finance.updateConcept.mockResolvedValue(concept({ active: false }));
    render(<ConceptsPanel canWrite />);

    await user.click(await screen.findByRole("button", { name: "Desactivar COLEGIATURA" }));

    await waitFor(() => expect(finance.updateConcept).toHaveBeenCalledWith(expect.any(String), "k1", expect.objectContaining({ active: false })));
    expect(await screen.findByRole("button", { name: "Reactivar COLEGIATURA" })).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent(/COLEGIATURA ya no se ofrece/);
  });

  it("marks a system concept and never offers to deactivate it (CC-404)", async () => {
    finance.listConcepts.mockResolvedValue([concept({ id: "k5", name: "RECARGO", kind: "late_fee" })]);
    render(<ConceptsPanel canWrite />);

    expect(await screen.findByText("Del sistema")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Editar RECARGO" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Desactivar RECARGO" })).not.toBeInTheDocument();
  });

  it("shows the server's reason when it refuses a change", async () => {
    const user = userEvent.setup();
    finance.updateConcept.mockRejectedValue(Object.assign(new Error("este concepto es del sistema: la facturación lo usa y no se puede desactivar"), { status: 400 }));
    render(<ConceptsPanel canWrite />);

    await user.click(await screen.findByRole("button", { name: "Desactivar COLEGIATURA" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/no se pudo guardar COLEGIATURA/i);
  });

  it("says so when the list cannot be read, and retries", async () => {
    const user = userEvent.setup();
    finance.listConcepts.mockRejectedValueOnce(new Error("HTTP 500"));
    render(<ConceptsPanel canWrite />);

    expect(await screen.findByRole("alert")).toHaveTextContent(/No se pudieron leer los conceptos/);
    await user.click(screen.getByRole("button", { name: "Reintentar" }));
    expect(await screen.findByText("COLEGIATURA")).toBeInTheDocument();
  });

  it("an empty catalog says what to do", async () => {
    finance.listConcepts.mockResolvedValue([]);
    render(<ConceptsPanel canWrite />);

    expect(await screen.findByText(/Aún no hay conceptos/)).toBeInTheDocument();
  });
});
