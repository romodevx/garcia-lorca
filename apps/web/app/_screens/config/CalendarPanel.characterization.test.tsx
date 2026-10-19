import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../config/api";
import { CalendarPanel } from "./CalendarPanel";

/*
 * Characterization tests (2026-10-09) for "Calendario escolar", frozen before
 * the Configuración académica redesign (behavior.md §6: load errors, empties,
 * range/vacation format, date rules, add/remove errors, activity undo A31/A32).
 */

vi.mock("../../config/api", async (importOriginal) => ({
  ConfigError: (await importOriginal<typeof import("../../config/api")>()).ConfigError,
  listNonWorkingPeriods: vi.fn(),
  createNonWorkingPeriod: vi.fn(),
  deleteNonWorkingPeriod: vi.fn(),
  listSchoolEvents: vi.fn(),
  createSchoolEvent: vi.fn(),
  deleteSchoolEvent: vi.fn(),
}));
const config = vi.mocked(configApi);

const cycle = { id: "c1", name: "2026-2027", status: "active" as const, startsOn: "2026-08-01", endsOn: "2027-06-30" };

const VACATION = { id: "d2", cycleId: "c1", label: "INVIERNO", kind: "vacation" as const, startsOn: "2026-12-21", endsOn: "2027-01-06" };
const FESTIVAL = { id: "ev1", cycleId: "c1", title: "FESTIVAL", description: "", startsOn: "2026-11-20", endsOn: "2026-11-20" };
const WEEK = { id: "ev2", cycleId: "c1", title: "EXÁMENES", description: "", startsOn: "2026-12-07", endsOn: "2026-12-11" };

function useFakeClock(): void {
  vi.useFakeTimers();
  Object.assign(globalThis, { jest: { advanceTimersByTime: (ms: number) => vi.advanceTimersByTime(ms) } });
}
function useRealClock(): void {
  Reflect.deleteProperty(globalThis, "jest");
  vi.useRealTimers();
}

beforeEach(() => {
  config.listNonWorkingPeriods.mockResolvedValue([]);
  config.listSchoolEvents.mockResolvedValue([]);
});
afterEach(useRealClock);
afterEach(() => vi.clearAllMocks());

describe("CalendarPanel — characterization", () => {
  it("loads both lists for the cycle and says when each is empty", async () => {
    render(<CalendarPanel cycle={cycle} />);

    expect(await screen.findByText("Todavía no hay días cerrados en este ciclo.")).toBeInTheDocument();
    expect(screen.getByText("Todavía no hay actividades en la agenda.")).toBeInTheDocument();
    expect(config.listNonWorkingPeriods).toHaveBeenCalledWith(expect.any(String), "c1");
    expect(config.listSchoolEvents).toHaveBeenCalledWith(expect.any(String), "c1");
  });

  it("says the calendar could not load", async () => {
    config.listNonWorkingPeriods.mockRejectedValue(new Error("down"));
    render(<CalendarPanel cycle={cycle} />);

    expect(await screen.findByRole("alert")).toHaveTextContent("No pudimos cargar el calendario.");
  });

  it("says the agenda could not load", async () => {
    config.listSchoolEvents.mockRejectedValue(new Error("down"));
    render(<CalendarPanel cycle={cycle} />);

    expect(await screen.findByRole("alert")).toHaveTextContent("No pudimos cargar la agenda.");
  });

  it("shows a range as 'inicio → fin' and marks vacations; an activity range uses the same arrow", async () => {
    config.listNonWorkingPeriods.mockResolvedValue([VACATION]);
    config.listSchoolEvents.mockResolvedValue([WEEK]);
    render(<CalendarPanel cycle={cycle} />);

    // T14-012: the month in words, as every other screen says a date.
    expect(await screen.findByText("21 dic 2026 → 6 ene 2027 · vacaciones")).toBeInTheDocument();
    expect(screen.getByText("7 dic 2026 → 11 dic 2026")).toBeInTheDocument();
  });

  it("refuses a closure whose last day is before its first, without calling the server", async () => {
    const user = userEvent.setup();
    render(<CalendarPanel cycle={cycle} />);
    await screen.findByText("Todavía no hay días cerrados en este ciclo.");

    await user.click(screen.getByRole("button", { name: "Nuevo día cerrado" }));
    await user.type(screen.getByLabelText("Nombre"), "Puente");
    await user.type(screen.getByLabelText("Del día"), "2026-11-20");
    await user.type(screen.getByLabelText("Al día (opcional)"), "2026-11-10");
    await user.keyboard("{Enter}");

    expect(await screen.findByText("El último día es antes que el primero.")).toBeInTheDocument();
    expect(config.createNonWorkingPeriod).not.toHaveBeenCalled();
  });

  it("refuses an activity whose end is before its start, without calling the server", async () => {
    const user = userEvent.setup();
    render(<CalendarPanel cycle={cycle} />);
    await screen.findByText("Todavía no hay actividades en la agenda.");

    await user.click(screen.getByRole("button", { name: "Nueva actividad" }));
    await user.type(screen.getByLabelText("Actividad"), "Semana");
    await user.type(screen.getByLabelText("Fecha"), "2026-11-20");
    await user.type(screen.getByLabelText("Hasta (opcional)"), "2026-11-10");
    await user.keyboard("{Enter}");

    expect(await screen.findByText("La fecha final es antes que la inicial.")).toBeInTheDocument();
    expect(config.createSchoolEvent).not.toHaveBeenCalled();
  });

  it("a failed closure shows the error's own message, or a generic one when the rejection is not an Error", async () => {
    const user = userEvent.setup();
    config.createNonWorkingPeriod.mockRejectedValueOnce(new Error("Ese día ya está cerrado."));
    config.createNonWorkingPeriod.mockRejectedValueOnce("nope");
    render(<CalendarPanel cycle={cycle} />);
    await screen.findByText("Todavía no hay días cerrados en este ciclo.");

    await user.click(screen.getByRole("button", { name: "Nuevo día cerrado" }));
    await user.type(screen.getByLabelText("Nombre"), "Puente");
    await user.type(screen.getByLabelText("Del día"), "2026-11-20");
    await user.keyboard("{Enter}");
    expect(await screen.findByText("Ese día ya está cerrado.")).toBeInTheDocument();

    await user.keyboard("{Enter}");
    expect(await screen.findByText("No se pudo agregar.")).toBeInTheDocument();
  });

  /*
   * SUSPECTED UX BUG, characterized as-is: a ConfigError with an empty body
   * reaches the screen as its fallback message "HTTP 500" instead of
   * "No se pudo agregar." / "No se pudo agregar la actividad.", because the
   * panel prints any Error's message.
   */
  it("a bodiless server failure is shown as 'HTTP {status}' (current behavior)", async () => {
    const user = userEvent.setup();
    config.createSchoolEvent.mockRejectedValue(new configApi.ConfigError(500, ""));
    render(<CalendarPanel cycle={cycle} />);
    await screen.findByText("Todavía no hay actividades en la agenda.");

    await user.click(screen.getByRole("button", { name: "Nueva actividad" }));
    await user.type(screen.getByLabelText("Actividad"), "Festival");
    await user.type(screen.getByLabelText("Fecha"), "2026-11-20");
    await user.keyboard("{Enter}");

    expect(await screen.findByRole("alert")).toHaveTextContent("HTTP 500");
  });

  it("removing an activity offers 'Se quitó {title}' and Deshacer brings it back without a DELETE", async () => {
    config.listSchoolEvents.mockResolvedValue([FESTIVAL]);
    const user = userEvent.setup();
    render(<CalendarPanel cycle={cycle} />);

    await user.click(await screen.findByRole("button", { name: "Quitar FESTIVAL" }));
    expect(screen.queryByText("FESTIVAL")).toBeNull();
    expect(screen.getByText("Se quitó FESTIVAL")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Deshacer" }));

    expect(await screen.findByText("FESTIVAL")).toBeInTheDocument();
    expect(config.deleteSchoolEvent).not.toHaveBeenCalled();
  });

  it("an activity removal reaches the server once the person closes the undo offer", async () => {
    useFakeClock();
    config.listSchoolEvents.mockResolvedValue([FESTIVAL]);
    config.deleteSchoolEvent.mockResolvedValue(undefined);
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<CalendarPanel cycle={cycle} />);

    await user.click(await screen.findByRole("button", { name: "Quitar FESTIVAL" }));
    await user.click(screen.getByRole("button", { name: "Cerrar aviso" }));

    expect(config.deleteSchoolEvent).toHaveBeenCalledWith(expect.any(String), "ev1", { keepalive: false });
  });

  // ISSUE-CC-105: removing an activity is another action, so the closed day
  // still on offer leaves the offer and reaches the server.
  it("removing an activity commits a closed day still on offer", async () => {
    config.listNonWorkingPeriods.mockResolvedValue([VACATION]);
    config.listSchoolEvents.mockResolvedValue([FESTIVAL]);
    config.deleteNonWorkingPeriod.mockResolvedValue(undefined);
    const user = userEvent.setup();
    render(<CalendarPanel cycle={cycle} />);

    await user.click(await screen.findByRole("button", { name: "Quitar INVIERNO" }));
    await user.click(await screen.findByRole("button", { name: "Quitar FESTIVAL" }));

    expect(config.deleteNonWorkingPeriod).toHaveBeenCalledWith(expect.any(String), "d2", { keepalive: false });
    expect(config.deleteSchoolEvent).not.toHaveBeenCalled();
    expect(screen.getAllByRole("button", { name: "Deshacer" })).toHaveLength(1);
  });

  it("says when a closed day could not be removed", async () => {
    useFakeClock();
    config.listNonWorkingPeriods.mockResolvedValue([VACATION]);
    config.deleteNonWorkingPeriod.mockRejectedValue(new Error("down"));
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<CalendarPanel cycle={cycle} />);

    await user.click(await screen.findByRole("button", { name: "Quitar INVIERNO" }));
    await user.click(screen.getByRole("button", { name: "Cerrar aviso" }));

    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("No se pudo quitar ese día."));
  });

  it("says when an activity could not be removed", async () => {
    useFakeClock();
    config.listSchoolEvents.mockResolvedValue([FESTIVAL]);
    config.deleteSchoolEvent.mockRejectedValue(new Error("down"));
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<CalendarPanel cycle={cycle} />);

    await user.click(await screen.findByRole("button", { name: "Quitar FESTIVAL" }));
    await user.click(screen.getByRole("button", { name: "Cerrar aviso" }));

    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("No se pudo quitar esa actividad."));
  });
});
