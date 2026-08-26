import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Cycle, Level } from "../../config/api";
import * as financeApi from "../../finance/api";
import { TuitionPanel, pesosToCents, centsToPesos } from "./TuitionPanel";

vi.mock("../../finance/api", () => ({
  listTuitionPlans: vi.fn(),
  upsertTuitionPlan: vi.fn(),
  runBilling: vi.fn(),
}));

const finance = vi.mocked(financeApi);

const cycle: Cycle = { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" };
const levels: Level[] = [
  { id: "lv1", name: "Kínder", sepCode: "K", enrollmentGranularity: "cycle" },
  { id: "lv2", name: "Primaria", sepCode: "P", enrollmentGranularity: "cycle" },
];

beforeEach(() => {
  finance.listTuitionPlans.mockResolvedValue([
    { id: "tp1", cycleId: "c1", levelId: "lv2", enrollmentCents: 500000, monthlyCents: 320000,
      months: 10, firstBilledMonth: 9, dueDay: 10, lateFeeCents: 15000, active: true },
  ]);
});
afterEach(() => vi.clearAllMocks());

describe("pesos and cents", () => {
  // Money is integer cents everywhere behind this screen, and the person typing
  // thinks in pesos. Getting this wrong by a factor of a hundred is the kind of
  // bug a school finds out about from a parent.
  it("round-trips without drifting", () => {
    expect(pesosToCents("3200")).toBe(320000);
    expect(pesosToCents("3,200.50")).toBe(320050);
    expect(pesosToCents("0.05")).toBe(5);
    expect(centsToPesos(320000)).toBe("3200.00");
    expect(centsToPesos(5)).toBe("0.05");
  });

  it("refuses what is not a number instead of silently reading zero", () => {
    expect(pesosToCents("")).toBeNull();
    expect(pesosToCents("abc")).toBeNull();
    expect(pesosToCents("-5")).toBeNull();
  });
});

describe("the tuition table", () => {
  it("shows a row per level, priced or not", async () => {
    render(<TuitionPanel cycle={cycle} levels={levels} />);

    // Primaria is priced; Kínder is not, and must say so rather than show zeros
    // that read like a free school.
    expect(await screen.findByText("Primaria")).toBeInTheDocument();
    const kinder = screen.getByRole("row", { name: /Kínder/ });
    expect(within(kinder).getByText(/Sin tabulador/)).toBeInTheDocument();
  });

  it("saves a level's prices in cents, from pesos typed by a person", async () => {
    const user = userEvent.setup();
    finance.upsertTuitionPlan.mockResolvedValue({
      id: "tp2", cycleId: "c1", levelId: "lv1", enrollmentCents: 400000, monthlyCents: 250000,
      months: 11, firstBilledMonth: 8, dueDay: 5, lateFeeCents: 10000, active: true,
    });
    render(<TuitionPanel cycle={cycle} levels={levels} />);
    await screen.findByText("Primaria");

    await user.click(within(screen.getByRole("row", { name: /Kínder/ })).getByRole("button", { name: /Definir|Editar/ }));
    const form = await screen.findByRole("form", { name: "Tabulador del nivel" });
    await user.type(within(form).getByLabelText("Inscripción"), "4000");
    await user.type(within(form).getByLabelText("Colegiatura mensual"), "2500");
    await user.clear(within(form).getByLabelText("Mensualidades"));
    await user.type(within(form).getByLabelText("Mensualidades"), "11");
    await user.clear(within(form).getByLabelText("Día límite"));
    await user.type(within(form).getByLabelText("Día límite"), "5");
    await user.type(within(form).getByLabelText("Recargo por mes vencido"), "100");
    await user.click(within(form).getByRole("button", { name: "Guardar tabulador" }));

    await waitFor(() => expect(finance.upsertTuitionPlan).toHaveBeenCalled());
    const sent = finance.upsertTuitionPlan.mock.calls[0]?.[1];
    expect(sent).toMatchObject({
      cycleId: "c1", levelId: "lv1",
      enrollmentCents: 400000, monthlyCents: 250000,
      months: 11, dueDay: 5, lateFeeCents: 10000,
    });
  });

  it("will not send a due day the calendar cannot honour", async () => {
    const user = userEvent.setup();
    render(<TuitionPanel cycle={cycle} levels={levels} />);
    await screen.findByText("Primaria");

    await user.click(within(screen.getByRole("row", { name: /Kínder/ })).getByRole("button", { name: /Definir|Editar/ }));
    const form = await screen.findByRole("form", { name: "Tabulador del nivel" });
    await user.type(within(form).getByLabelText("Colegiatura mensual"), "2500");
    await user.clear(within(form).getByLabelText("Día límite"));
    await user.type(within(form).getByLabelText("Día límite"), "31");
    await user.click(within(form).getByRole("button", { name: "Guardar tabulador" }));

    // The 31st does not exist in February. The server refuses it too, but a
    // person should not have to make a round trip to learn that.
    expect(await screen.findByText(/entre 1 y 28/)).toBeInTheDocument();
    expect(finance.upsertTuitionPlan).not.toHaveBeenCalled();
  });
});

describe("running the month", () => {
  it("reports what it did, skipped included", async () => {
    const user = userEvent.setup();
    finance.runBilling.mockResolvedValue({
      tuitionCreated: 30, enrollmentCreated: 10, lateFeesCreated: 4, skipped: 6, levelsWithoutPlan: [],
    });
    render(<TuitionPanel cycle={cycle} levels={levels} />);
    await screen.findByText("Primaria");

    await user.click(screen.getByRole("button", { name: /Generar cargos/ }));

    // "Ya existían" is what makes a second press legible instead of frightening.
    expect(await screen.findByText(/30 colegiaturas/)).toBeInTheDocument();
    expect(screen.getByText(/6 ya existían/)).toBeInTheDocument();
  });

  it("names the levels nobody priced, because a level billed at zero is silent", async () => {
    const user = userEvent.setup();
    finance.runBilling.mockResolvedValue({
      tuitionCreated: 0, enrollmentCreated: 0, lateFeesCreated: 0, skipped: 0,
      levelsWithoutPlan: ["Kínder", "Preparatoria"],
    });
    render(<TuitionPanel cycle={cycle} levels={levels} />);
    await screen.findByText("Primaria");

    await user.click(screen.getByRole("button", { name: /Generar cargos/ }));

    expect(await screen.findByText(/Kínder, Preparatoria/)).toBeInTheDocument();
  });
});
