import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as commsApi from "../../comms/api";
import * as configApi from "../../config/api";
import * as studentsApi from "../../students/api";
import { AnnouncementsCompose } from "./AnnouncementsCompose";

/*
 * What only the new view of /avisos adds (PLAN-redesign-avisos §10). The
 * behavior contract itself runs against both views in the characterization
 * suites; these cover the layout's own promises.
 */

vi.mock("../../comms/api", () => ({
  announce: vi.fn(),
  getBirthdayTemplate: vi.fn().mockResolvedValue({ title: "¡Feliz cumpleaños, {nombre}!", body: "Hola {nombre}", isDefault: true, updatedAt: "" }),
  updateBirthdayTemplate: vi.fn(),
}));
vi.mock("../../config/api", () => ({ listCycles: vi.fn(), listLevels: vi.fn(), listLabelledGroups: vi.fn() }));
vi.mock("../../students/api", () => ({ listStudents: vi.fn() }));

const comms = vi.mocked(commsApi);
const config = vi.mocked(configApi);
const students = vi.mocked(studentsApi);

const ACTIVE = { id: "c2", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" } as const;
const PRIMARIA = { id: "lv1", name: "Primaria", sepCode: "PRIM", enrollmentGranularity: "cycle" } as const;

beforeEach(() => {
  config.listCycles.mockResolvedValue([ACTIVE]);
  config.listLevels.mockResolvedValue([PRIMARIA]);
  config.listLabelledGroups.mockResolvedValue([
    { id: "g1", cycleId: "c2", gradeLevelId: "gl1", name: "A", capacity: 30, label: "1° PRIMARIA · A", gradeName: "1°", levelId: "lv1", ordinal: 1 },
    { id: "g2", cycleId: "c2", gradeLevelId: "gl1", name: "B", capacity: 30, label: "1° PRIMARIA · B", gradeName: "1°", levelId: "lv1", ordinal: 1 },
  ]);
  students.listStudents.mockResolvedValue([
    { id: "s1", curp: "C1", firstName: "Valentina", lastName: "Cruz", level: "Primaria", levelId: "lv1", gradeGroup: "1° A", guardianName: "", admissionStatus: "enrolled" },
  ]);
  comms.announce.mockResolvedValue({ created: 28, audience: 28 });
});
afterEach(() => vi.clearAllMocks());

const renderView = () => render(<AnnouncementsCompose onBack={() => {}} />);

describe("AnnouncementsCompose", () => {
  it("files the groups under their level and shows them short", async () => {
    renderView();
    const row = await screen.findByRole("button", { name: "1° PRIMARIA · A" });
    expect(row).toHaveTextContent(/^1° A$/);
    expect(row.closest(".announce-level")).toHaveTextContent("Primaria");
  });

  // E2: one node of each at any width; CSS moves them, never a copy.
  it("has one send button, one alert and one status", async () => {
    const user = userEvent.setup();
    comms.announce.mockRejectedValueOnce(new Error("se cayó la red"));
    renderView();
    await user.click(await screen.findByRole("button", { name: "1° PRIMARIA · A" }));
    await user.type(screen.getByLabelText("Título"), "Junta");
    await user.type(screen.getByLabelText("Mensaje"), "Jueves");
    expect(screen.getAllByRole("button", { name: /Enviar aviso/ })).toHaveLength(1);
    await user.click(screen.getByRole("button", { name: /Enviar aviso/ }));
    expect(await screen.findAllByRole("alert")).toHaveLength(1);
    await user.click(screen.getByRole("button", { name: /Enviar aviso/ }));
    expect(await screen.findAllByRole("status")).toHaveLength(1);
  });

  // D12
  it("takes the focus to the alert when a send is refused", async () => {
    const user = userEvent.setup();
    renderView();
    await screen.findByRole("button", { name: "1° PRIMARIA · A" });
    await user.click(screen.getByRole("button", { name: /Enviar aviso/ }));
    expect(await screen.findByRole("alert")).toHaveFocus();
    expect(screen.getByRole("alert")).toHaveTextContent("Un aviso necesita título y mensaje.");
  });

  it("says who it goes to beside the button", async () => {
    const user = userEvent.setup();
    renderView();
    await user.click(await screen.findByRole("button", { name: "1° PRIMARIA · B" }));
    await user.click(screen.getByRole("button", { name: "1° PRIMARIA · A" }));
    expect(screen.getByText(/^Para/)).toHaveTextContent("Para 1° PRIMARIA · B y 1° PRIMARIA · A");
  });

  it("names the cycle when the audience is the whole school", async () => {
    const user = userEvent.setup();
    renderView();
    await screen.findByRole("button", { name: "1° PRIMARIA · A" });
    await user.click(screen.getByRole("button", { name: "Toda la escuela" }));
    expect(screen.getByText("Toda la escuela · ciclo 2026-2027")).toBeInTheDocument();
  });

  // D1
  it("does not send when Enter is pressed in the pupil search", async () => {
    const user = userEvent.setup();
    renderView();
    await screen.findByRole("button", { name: "1° PRIMARIA · A" });
    await user.click(screen.getByRole("button", { name: "Un alumno" }));
    await user.type(screen.getByLabelText("Título"), "Junta");
    await user.type(screen.getByLabelText("Mensaje"), "Jueves");
    await user.type(screen.getByRole("searchbox", { name: "Buscar alumno" }), "Cruz{Enter}");
    expect(comms.announce).not.toHaveBeenCalled();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  // D11
  it("says it is loading the groups until they arrive", async () => {
    let arrive: (groups: configApi.LabelledGroup[]) => void = () => {};
    config.listLabelledGroups.mockReturnValue(new Promise((resolve) => { arrive = resolve; }));
    renderView();
    await waitFor(() => expect(config.listLabelledGroups).toHaveBeenCalled());
    expect(screen.getAllByText("Cargando…").length).toBeGreaterThan(0);
    arrive([]);
    expect(await screen.findByText("Este ciclo no tiene grupos.")).toBeInTheDocument();
  });

  it("does not keep loading groups forever when there is no cycle", async () => {
    config.listCycles.mockResolvedValue([]);
    renderView();
    expect(await screen.findByText("Este ciclo no tiene grupos.")).toBeInTheDocument();
  });

  // E3: the form is the two-pane grid; the greeting has its own form, outside.
  it("keeps the birthday greeting outside the announcement's form", async () => {
    renderView();
    await screen.findByDisplayValue("¡Feliz cumpleaños, {nombre}!");
    const send = screen.getByRole("button", { name: /Enviar aviso/ }).closest("form");
    const greeting = screen.getByRole("form", { name: "Felicitación de cumpleaños" });
    expect(send).not.toBeNull();
    expect(send).not.toContainElement(greeting);
    expect(send).toContainElement(screen.getByRole("region", { name: "Destinatarios" }));
  });
});
