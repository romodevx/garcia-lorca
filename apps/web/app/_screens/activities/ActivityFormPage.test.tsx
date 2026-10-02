import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as api from "../../activities/api";
import { ActivityFormPage } from "./ActivityFormPage";

vi.mock("../../activities/api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../../activities/api")>();
  return { ...real, createActivity: vi.fn(), updateActivity: vi.fn() };
});
const m = vi.mocked(api);
afterEach(() => vi.clearAllMocks());

const OPT = "0f000000-0000-0000-0000-000000000001";
const swim: api.Activity = {
  id: "a1000000-0000-0000-0000-000000000001", name: "Natación", enrolledCount: 2, periodicity: "monthly", active: true, lateFeeCents: 5000,
  schedule: [{ weekday: 2, start: "15:00", end: "16:30" }], billingStartsOn: "2026-10-01", firstDueOn: "2026-10-10",
  options: [{ id: OPT, daysPerWeek: 1, amountCents: 60000 }],
};

function open(activity?: api.Activity): { onSaved: ReturnType<typeof vi.fn>; onClose: ReturnType<typeof vi.fn> } {
  const onSaved = vi.fn();
  const onClose = vi.fn();
  render(<ActivityFormPage activity={activity} today="2026-10-02" onClose={onClose} onSaved={onSaved} />);
  return { onSaved, onClose };
}

const slot = (i: number): HTMLElement => screen.getByRole("group", { name: `Horario ${i}` });
const option = (i: number): HTMLElement => screen.getByRole("group", { name: `Precio ${i}` });

describe("ActivityFormPage (T17)", () => {
  it("saving empty marks every missing field and focuses the first; the button is never disabled", async () => {
    const user = userEvent.setup();
    open();
    const save = screen.getByRole("button", { name: /Guardar actividad/ });
    expect(save).toBeEnabled();
    await user.click(save);
    expect(m.createActivity).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Nombre")).toHaveAttribute("aria-invalid", "true");
    expect(within(slot(1)).getByLabelText("Día")).toHaveAttribute("aria-invalid", "true");
    expect(within(option(1)).getByLabelText("Monto")).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByLabelText("Nombre")).toHaveFocus();
  });

  it("creates an activity with two days of different hours and two price options", async () => {
    const user = userEvent.setup();
    m.createActivity.mockResolvedValue(swim);
    const { onSaved } = open();
    await user.type(screen.getByLabelText("Nombre"), "Natación");
    await user.selectOptions(within(slot(1)).getByLabelText("Día"), "Martes");
    await user.selectOptions(within(slot(1)).getByLabelText("Inicio"), "15:00");
    await user.selectOptions(within(slot(1)).getByLabelText("Fin"), "16:30");
    await user.click(screen.getByRole("button", { name: /Agregar día/ }));
    await user.selectOptions(within(slot(2)).getByLabelText("Día"), "Jueves");
    await user.selectOptions(within(slot(2)).getByLabelText("Inicio"), "16:00");
    await user.selectOptions(within(slot(2)).getByLabelText("Fin"), "17:00");
    await user.selectOptions(screen.getByLabelText("Periodicidad"), "Semanal");
    await user.type(screen.getByLabelText("Primera fecha límite"), "2026-10-09");
    await user.selectOptions(within(option(1)).getByLabelText("Esquema"), "1 día por semana");
    await user.type(within(option(1)).getByLabelText("Monto"), "600");
    await user.click(screen.getByRole("button", { name: /Agregar precio/ }));
    await user.selectOptions(within(option(2)).getByLabelText("Esquema"), "Cuota fija (sin importar los días)");
    await user.type(within(option(2)).getByLabelText("Monto"), "900");
    await user.click(screen.getByRole("button", { name: /Guardar actividad/ }));

    expect(m.createActivity).toHaveBeenCalledWith(expect.any(String), {
      name: "NATACIÓN",
      schedule: [{ weekday: 2, start: "15:00", end: "16:30" }, { weekday: 4, start: "16:00", end: "17:00" }],
      periodicity: "weekly",
      billingStartsOn: "2026-10-02",
      firstDueOn: "2026-10-09",
      lateFeeCents: 0,
      active: true,
      options: [{ daysPerWeek: 1, amountCents: 60000 }, { daysPerWeek: null, amountCents: 90000 }],
    });
    expect(onSaved).toHaveBeenCalledWith(expect.stringMatching(/guardada/));
  });

  it("removes a row", async () => {
    const user = userEvent.setup();
    open();
    await user.click(screen.getByRole("button", { name: /Agregar día/ }));
    await user.click(within(slot(2)).getByRole("button", { name: /Quitar/ }));
    expect(screen.queryByRole("group", { name: "Horario 2" })).not.toBeInTheDocument();
  });

  it("an end before the start is said when leaving the field", async () => {
    const user = userEvent.setup();
    open();
    await user.selectOptions(within(slot(1)).getByLabelText("Inicio"), "15:00");
    await user.selectOptions(within(slot(1)).getByLabelText("Fin"), "14:00");
    await user.tab();
    expect(within(slot(1)).getByText(/después del inicio/)).toBeInTheDocument();
  });

  it("edits an activity, keeping the id of the option it already had", async () => {
    const user = userEvent.setup();
    m.updateActivity.mockResolvedValue(swim);
    open(swim);
    const amount = within(option(1)).getByLabelText("Monto");
    await user.clear(amount);
    await user.type(amount, "650");
    await user.click(screen.getByRole("button", { name: /Guardar actividad/ }));
    expect(m.updateActivity).toHaveBeenCalledWith(expect.any(String), swim.id, expect.objectContaining({
      lateFeeCents: 5000,
      options: [{ id: OPT, daysPerWeek: 1, amountCents: 65000 }],
    }));
  });

  it("shows the gateway's refusal as it came", async () => {
    const user = userEvent.setup();
    m.updateActivity.mockRejectedValue(new Error("Esa opción la usa un alumno inscrito."));
    open(swim);
    await user.click(screen.getByRole("button", { name: /Guardar actividad/ }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Esa opción la usa un alumno inscrito.");
  });
});
