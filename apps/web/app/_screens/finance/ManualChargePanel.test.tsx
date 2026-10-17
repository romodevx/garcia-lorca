import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as financeApi from "../../finance/api";
import { ManualChargePanel, type PupilOption } from "./ManualChargePanel";

vi.mock("../../finance/api", () => ({ listConcepts: vi.fn(), createCharge: vi.fn() }));

const finance = vi.mocked(financeApi);

const PUPILS: PupilOption[] = [
  { id: "s1", label: "CRUZ VALENTINA · 3° PRIMARIA · A" },
  { id: "s2", label: "SÁNCHEZ MATEO · 1° SECUNDARIA · B" },
];
const CREDENCIAL: financeApi.ChargeConcept = { id: "k9", name: "CREDENCIAL", defaultAmountCents: 8000, recurrence: "once", active: true };
const CREATED: financeApi.Charge = {
  id: "ch9", studentId: "s1", cycleId: "c1", conceptId: "k9", description: "CREDENCIAL", amountCents: 8000,
  discountCents: 0, dueDate: "2026-10-17", status: "pending", paidCents: 0, remainingCents: 8000,
};

function setup(): { onCreated: ReturnType<typeof vi.fn> } {
  const onCreated = vi.fn();
  render(<ManualChargePanel cycleId="c1" pupils={PUPILS} today="2026-10-17" onCreated={onCreated} onClose={vi.fn()} />);
  return { onCreated };
}

beforeEach(() => {
  finance.listConcepts.mockResolvedValue([CREDENCIAL]);
  finance.createCharge.mockResolvedValue(CREATED);
});
afterEach(() => vi.clearAllMocks());

describe("ManualChargePanel (CC-203 + CC-111)", () => {
  it("charges a catalog concept to one pupil, prefilled with its amount and today", async () => {
    const user = userEvent.setup();
    const { onCreated } = setup();

    await user.type(screen.getByLabelText(/^Alumno/), "CRUZ VALENTINA · 3° PRIMARIA · A");
    await user.selectOptions(await screen.findByLabelText(/^Concepto/), "k9");
    expect(screen.getByLabelText(/^Monto/)).toHaveValue("80.00");
    expect(screen.getByLabelText(/^Vence/)).toHaveValue("2026-10-17");
    await user.click(screen.getByRole("button", { name: "Crear cargo" }));

    await waitFor(() => expect(finance.createCharge).toHaveBeenCalledWith(expect.any(String), {
      studentId: "s1", cycleId: "c1", conceptId: "k9", amountCentsOverride: 8000, descriptionOverride: "", dueDate: "2026-10-17",
    }));
    expect(onCreated).toHaveBeenCalledWith(CREATED, "Cargo de CREDENCIAL creado para CRUZ VALENTINA · 3° PRIMARIA · A.");
  });

  it("reads only the active concepts (the catalog is its source)", async () => {
    setup();

    await screen.findByLabelText(/^Concepto/);
    expect(finance.listConcepts).toHaveBeenCalledWith(expect.any(String));
  });

  it("asks for what is missing on submit and never disables the button", async () => {
    const user = userEvent.setup();
    setup();

    await screen.findByLabelText(/^Concepto/);
    const create = screen.getByRole("button", { name: "Crear cargo" });
    expect(create).toBeEnabled();
    await user.click(create);

    expect(await screen.findAllByText("Falta este dato")).toHaveLength(3);
    expect(finance.createCharge).not.toHaveBeenCalled();
  });

  it("refuses a pupil typed but not picked from the list", async () => {
    const user = userEvent.setup();
    setup();

    await user.type(screen.getByLabelText(/^Alumno/), "CRUZ");
    await user.selectOptions(await screen.findByLabelText(/^Concepto/), "k9");
    await user.click(screen.getByRole("button", { name: "Crear cargo" }));

    expect(await screen.findByText("Elige un alumno de la lista.")).toBeInTheDocument();
    expect(finance.createCharge).not.toHaveBeenCalled();
  });

  it("refuses a zero amount", async () => {
    const user = userEvent.setup();
    setup();

    await user.type(screen.getByLabelText(/^Alumno/), "CRUZ VALENTINA · 3° PRIMARIA · A");
    await user.selectOptions(await screen.findByLabelText(/^Concepto/), "k9");
    await user.clear(screen.getByLabelText(/^Monto/));
    await user.type(screen.getByLabelText(/^Monto/), "0");
    await user.click(screen.getByRole("button", { name: "Crear cargo" }));

    expect(await screen.findByText("El monto debe ser mayor que cero.")).toBeInTheDocument();
    expect(finance.createCharge).not.toHaveBeenCalled();
  });

  it("with an empty catalog, points to Conceptos de cobro", async () => {
    finance.listConcepts.mockResolvedValue([]);
    setup();

    expect(await screen.findByText(/da de alta un concepto en Conceptos de cobro/)).toBeInTheDocument();
  });

  it.each([
    [409, "Este alumno ya tiene un cargo de ese concepto en el ciclo."],
    [403, "Tu cuenta no tiene permiso para crear cargos."],
  ])("says what a %i means", async (status, message) => {
    const user = userEvent.setup();
    finance.createCharge.mockRejectedValue(Object.assign(new Error("refused"), { status }));
    setup();

    await user.type(screen.getByLabelText(/^Alumno/), "CRUZ VALENTINA · 3° PRIMARIA · A");
    await user.selectOptions(await screen.findByLabelText(/^Concepto/), "k9");
    await user.click(screen.getByRole("button", { name: "Crear cargo" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(message);
  });

  it("says when the server could not be reached", async () => {
    const user = userEvent.setup();
    finance.createCharge.mockRejectedValue(new TypeError("Failed to fetch"));
    setup();

    await user.type(screen.getByLabelText(/^Alumno/), "CRUZ VALENTINA · 3° PRIMARIA · A");
    await user.selectOptions(await screen.findByLabelText(/^Concepto/), "k9");
    await user.click(screen.getByRole("button", { name: "Crear cargo" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/No hubo conexión/);
  });

  it("keeps the form when the server refuses", async () => {
    const user = userEvent.setup();
    finance.createCharge.mockRejectedValue(new Error("HTTP 500"));
    const { onCreated } = setup();

    await user.type(screen.getByLabelText(/^Alumno/), "CRUZ VALENTINA · 3° PRIMARIA · A");
    await user.selectOptions(await screen.findByLabelText(/^Concepto/), "k9");
    await user.click(screen.getByRole("button", { name: "Crear cargo" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/No se pudo crear el cargo/);
    expect(screen.getByLabelText(/^Alumno/)).toHaveValue("CRUZ VALENTINA · 3° PRIMARIA · A");
    expect(onCreated).not.toHaveBeenCalled();
  });
});
