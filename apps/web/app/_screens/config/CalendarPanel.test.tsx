import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../config/api";
import { CalendarPanel } from "./CalendarPanel";

vi.mock("../../config/api", () => ({
  listNonWorkingPeriods: vi.fn(),
  createNonWorkingPeriod: vi.fn(),
  deleteNonWorkingPeriod: vi.fn(),
  listSchoolEvents: vi.fn(),
  createSchoolEvent: vi.fn(),
  deleteSchoolEvent: vi.fn(),
}));
const config = vi.mocked(configApi);

const cycle = { id: "c1", name: "2026-2027", status: "active" as const, startsOn: "2026-08-01", endsOn: "2027-06-30" };

beforeEach(() => {
  config.listSchoolEvents.mockResolvedValue([]);
  config.listNonWorkingPeriods.mockResolvedValue([
    { id: "per-1", cycleId: "c1", label: "Independencia", kind: "holiday", startsOn: "2026-09-16", endsOn: "2026-09-16" },
  ]);
});
afterEach(() => vi.clearAllMocks());

describe("CalendarPanel", () => {
  it("lists the days the school is closed", async () => {
    render(<CalendarPanel cycle={cycle} />);

    expect(await screen.findByText("Independencia")).toBeInTheDocument();
  });

  // A single holiday is one date. Making a person type it twice is how a
  // calendar stops being maintained, and an unmaintained calendar quietly moves
  // every early-payment deadline.
  it("adds a single-day holiday from one date", async () => {
    const user = userEvent.setup();
    config.createNonWorkingPeriod.mockResolvedValue({
      id: "per-2", cycleId: "c1", label: "Revolución", kind: "holiday", startsOn: "2026-11-20", endsOn: "2026-11-20",
    });
    render(<CalendarPanel cycle={cycle} />);
    await screen.findByText("Independencia");

    await user.click(screen.getByRole("button", { name: "Nuevo día cerrado" }));
    await user.type(screen.getByLabelText("Nombre"), "Revolución");
    await user.type(screen.getByLabelText("Del día"), "2026-11-20");
    await user.keyboard("{Enter}");

    await waitFor(() => expect(config.createNonWorkingPeriod).toHaveBeenCalled());
    const sent = config.createNonWorkingPeriod.mock.calls[0]?.[1];
    // Se captura en mayúsculas: el nombre del día cerrado es dato de catálogo.
    expect(sent?.label).toBe("REVOLUCIÓN");
    expect(sent?.startsOn).toBe("2026-11-20");
    expect(sent?.endsOn).toBeUndefined();
  });

  it("refuses to add a closure with no name", async () => {
    const user = userEvent.setup();
    render(<CalendarPanel cycle={cycle} />);
    await screen.findByText("Independencia");

    await user.click(screen.getByRole("button", { name: "Nuevo día cerrado" }));
    await user.type(screen.getByLabelText("Del día"), "2026-11-20");
    await user.keyboard("{Enter}");

    expect(config.createNonWorkingPeriod).not.toHaveBeenCalled();
  });

  /**
   * Quitar ya no borra en seco. Se va de la lista al instante —la interfaz no
   * duda de quien la usa— y hay ocho segundos para arrepentirse antes de que el
   * servidor se entere.
   */
  it("quitar un día ofrece deshacer, y deshacer lo devuelve", async () => {
    const user = userEvent.setup();
    render(<CalendarPanel cycle={cycle} />);
    await screen.findByText("Independencia");

    await user.click(screen.getByRole("button", { name: /Quitar Independencia/ }));
    expect(screen.queryByText("Independencia")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Deshacer" }));

    expect(await screen.findByText("Independencia")).toBeInTheDocument();
    expect(config.deleteNonWorkingPeriod).not.toHaveBeenCalled();
  });

  it("si nadie deshace, el borrado sí llega al servidor", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    config.deleteNonWorkingPeriod.mockResolvedValue(undefined);
    render(<CalendarPanel cycle={cycle} />);
    await screen.findByText("Independencia");

    await user.click(screen.getByRole("button", { name: /Quitar Independencia/ }));
    await act(async () => { vi.advanceTimersByTime(8000); });

    expect(config.deleteNonWorkingPeriod).toHaveBeenCalledWith(expect.anything(), "per-1");
    vi.useRealTimers();
  });

  // The agenda lives on the same screen and means something different: a
  // festival on the 10th does not mean nobody could pay on the 10th.
  it("adds an activity without closing the school", async () => {
    const user = userEvent.setup();
    config.createSchoolEvent.mockResolvedValue({
      id: "ev-1", cycleId: "c1", title: "Festival de primavera",
      description: "", startsOn: "2026-09-20", endsOn: "2026-09-20",
    });
    render(<CalendarPanel cycle={cycle} />);
    await screen.findByText("Independencia");

    await user.click(screen.getByRole("button", { name: "Nueva actividad" }));
    await user.type(screen.getByLabelText("Actividad"), "Festival de primavera");
    await user.type(screen.getByLabelText("Fecha"), "2026-09-20");
    await user.keyboard("{Enter}");

    await waitFor(() => expect(config.createSchoolEvent).toHaveBeenCalled());
    // And it never touches the closed days, which are what move a due date.
    expect(config.createNonWorkingPeriod).not.toHaveBeenCalled();
  });

  it("lists the agenda beside the closed days", async () => {
    config.listSchoolEvents.mockResolvedValue([
      { id: "ev-1", cycleId: "c1", title: "Junta de padres", description: "", startsOn: "2026-09-10", endsOn: "2026-09-10" },
    ]);
    render(<CalendarPanel cycle={cycle} />);

    expect(await screen.findByText("Junta de padres")).toBeInTheDocument();
    expect(screen.getByText("Independencia")).toBeInTheDocument();
  });
});

describe("la captura del calendario", () => {
  it("las fechas de un día cerrado se eligen en calendario, no se teclean", async () => {
    const user = userEvent.setup();
    render(<CalendarPanel cycle={cycle} />);
    await screen.findByText("Independencia");
    await user.click(screen.getByRole("button", { name: "Nuevo día cerrado" }));

    expect(screen.getByLabelText("Del día")).toHaveAttribute("type", "date");
    expect(screen.getByLabelText("Al día (opcional)")).toHaveAttribute("type", "date");
  });

  it("las fechas de una actividad también", async () => {
    const user = userEvent.setup();
    render(<CalendarPanel cycle={cycle} />);
    await screen.findByText("Independencia");
    await user.click(screen.getByRole("button", { name: "Nueva actividad" }));

    expect(screen.getByLabelText("Fecha")).toHaveAttribute("type", "date");
    expect(screen.getByLabelText("Hasta (opcional)")).toHaveAttribute("type", "date");
  });

  it("el nombre del día cerrado se captura en mayúsculas", async () => {
    const user = userEvent.setup();
    render(<CalendarPanel cycle={cycle} />);
    await screen.findByText("Independencia");
    await user.click(screen.getByRole("button", { name: "Nuevo día cerrado" }));

    const nombre = screen.getByLabelText("Nombre");
    await user.type(nombre, "revolución");
    expect(nombre).toHaveValue("REVOLUCIÓN");
  });

  it("lo obligatorio se marca en rojo al guardar, y se limpia al escribir", async () => {
    const user = userEvent.setup();
    render(<CalendarPanel cycle={cycle} />);
    await screen.findByText("Independencia");
    await user.click(screen.getByRole("button", { name: "Nuevo día cerrado" }));
    await user.keyboard("{Enter}");

    expect(screen.getByLabelText("Nombre")).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByLabelText("Del día")).toHaveAttribute("aria-invalid", "true");
    // Lo opcional nunca se reclama.
    expect(screen.getByLabelText("Al día (opcional)")).not.toHaveAttribute("aria-invalid", "true");
    expect(config.createNonWorkingPeriod).not.toHaveBeenCalled();

    await user.type(screen.getByLabelText("Nombre"), "R");
    expect(screen.getByLabelText("Nombre")).not.toHaveAttribute("aria-invalid", "true");
    expect(screen.getByLabelText("Del día")).toHaveAttribute("aria-invalid", "true");
  });
});

/**
 * Quitar dos días seguidos.
 *
 * Con un solo pendiente guardado, el segundo "Quitar" sustituía al primero: la
 * lista lo mostraba ido y el servidor nunca se enteraba. Para un día cerrado eso
 * deja mal la ventana de pronto pago de TODAS las familias, en silencio.
 */
describe("quitar varios días dentro del mismo plazo", () => {
  it("borra los dos, no sólo el último", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    config.deleteNonWorkingPeriod.mockResolvedValue(undefined);
    config.listNonWorkingPeriods.mockResolvedValue([
      { id: "per-1", cycleId: "c1", label: "INDEPENDENCIA", kind: "holiday", startsOn: "2026-09-16", endsOn: "2026-09-16" },
      { id: "per-2", cycleId: "c1", label: "REVOLUCIÓN", kind: "holiday", startsOn: "2026-11-20", endsOn: "2026-11-20" },
    ]);
    render(<CalendarPanel cycle={cycle} />);
    await screen.findByText("INDEPENDENCIA");

    await user.click(screen.getByRole("button", { name: /Quitar INDEPENDENCIA/ }));
    await act(async () => { vi.advanceTimersByTime(3000); });
    await user.click(screen.getByRole("button", { name: /Quitar REVOLUCIÓN/ }));
    await act(async () => { vi.advanceTimersByTime(8000); });

    expect(config.deleteNonWorkingPeriod).toHaveBeenCalledWith(expect.anything(), "per-1");
    expect(config.deleteNonWorkingPeriod).toHaveBeenCalledWith(expect.anything(), "per-2");
    vi.useRealTimers();
  });

  // Irse de la pantalla dentro del plazo no puede cancelar un borrado que la
  // lista ya mostró como hecho.
  it("si la pantalla se cierra, el borrado se manda de inmediato", async () => {
    const user = userEvent.setup();
    config.deleteNonWorkingPeriod.mockResolvedValue(undefined);
    const { unmount } = render(<CalendarPanel cycle={cycle} />);
    await screen.findByText("Independencia");

    await user.click(screen.getByRole("button", { name: /Quitar Independencia/ }));
    unmount();

    expect(config.deleteNonWorkingPeriod).toHaveBeenCalledWith(expect.anything(), "per-1");
  });
});
