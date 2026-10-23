import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as commsApi from "../../comms/api";
import * as configApi from "../../config/api";
import * as familiesApi from "../../families/api";
import * as studentsApi from "../../students/api";
import { AnnouncementsCompose } from "./AnnouncementsCompose";

/*
 * ADR 0041 (D4, Avisos): "Familias" as an audience, only with module_families.
 * The gateway resolves each family to its pupils and sends one push per parent.
 */

vi.mock("../../comms/api", () => ({
  announce: vi.fn(),
  getBirthdayTemplate: vi.fn().mockResolvedValue({ title: "¡Feliz cumpleaños, {nombre}!", body: "Hola {nombre}", isDefault: true, updatedAt: "" }),
  updateBirthdayTemplate: vi.fn(),
}));
vi.mock("../../config/api", () => ({ listCycles: vi.fn(), listLevels: vi.fn(), listLabelledGroups: vi.fn() }));
vi.mock("../../students/api", () => ({ listStudentsWithRooms: vi.fn() }));
vi.mock("../../families/api", () => ({ listFamilies: vi.fn() }));

const comms = vi.mocked(commsApi);
const config = vi.mocked(configApi);
const students = vi.mocked(studentsApi);
const families = vi.mocked(familiesApi);

const pupil = (id: string, lastName: string, firstName: string): studentsApi.Student => ({
  id, curp: id, firstName, lastName, maternalLastName: "TORRES", level: "Primaria", levelId: "lv1", gradeGroup: "A", guardianName: "", admissionStatus: "enrolled",
});
const family = (id: string, name: string, studentIds: string[]): familiesApi.Family => ({ id, name, studentIds, createdAt: "" });

beforeEach(() => {
  config.listCycles.mockResolvedValue([{ id: "c2", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" }]);
  config.listLevels.mockResolvedValue([]);
  config.listLabelledGroups.mockResolvedValue([]);
  students.listStudentsWithRooms.mockResolvedValue([pupil("s1", "GARCÍA", "ANA"), pupil("s2", "GARCÍA", "LUIS"), pupil("s3", "PÉREZ", "EVA")]);
  families.listFamilies.mockResolvedValue([family("f1", "GARCÍA TORRES", ["s1", "s2"]), family("f2", "PÉREZ TORRES", ["s3"])]);
  comms.announce.mockResolvedValue({ created: 3, audience: 3 });
});
afterEach(() => vi.clearAllMocks());

const pane = (): HTMLElement => screen.getByRole("region", { name: "Destinatarios" });

describe("AnnouncementsCompose — Familias", () => {
  it("off: no Familias audience and nothing read", async () => {
    render(<AnnouncementsCompose onBack={() => {}} />);
    await screen.findByRole("button", { name: "Grupos" });
    expect(screen.queryByRole("button", { name: "Familias" })).toBeNull();
    expect(families.listFamilies).not.toHaveBeenCalled();
  });

  it("the lead names families only when the audience is offered", async () => {
    const { unmount } = render(<AnnouncementsCompose onBack={() => {}} />);
    expect(await screen.findByText("Un mensaje para un alumno, un grupo, un nivel o toda la escuela")).toBeInTheDocument();
    unmount();
    render(<AnnouncementsCompose onBack={() => {}} families />);
    expect(await screen.findByText("Un mensaje para un alumno, una familia, un grupo, un nivel o toda la escuela")).toBeInTheDocument();
  });

  it("on: the families are read once, only when the audience is chosen", async () => {
    const user = userEvent.setup();
    render(<AnnouncementsCompose onBack={() => {}} families />);
    await screen.findByRole("button", { name: "Grupos" });
    expect(families.listFamilies).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Familias" }));
    expect(await screen.findByRole("button", { name: "Familia GARCÍA TORRES" })).toBeInTheDocument();
    for (const name of ["Grupos", "Familias", "Un alumno", "Familias"]) await user.click(screen.getByRole("button", { name }));
    expect(families.listFamilies).toHaveBeenCalledTimes(1);
  });

  it("each family shows its children; the search finds a family by a child's name", async () => {
    const user = userEvent.setup();
    render(<AnnouncementsCompose onBack={() => {}} families />);
    await user.click(await screen.findByRole("button", { name: "Familias" }));
    const garcia = await screen.findByRole("button", { name: "Familia GARCÍA TORRES" });
    expect(garcia).toHaveTextContent("GARCÍA TORRES ANA");
    await user.type(screen.getByRole("searchbox", { name: "Buscar familia" }), "eva");
    expect(screen.queryByRole("button", { name: "Familia GARCÍA TORRES" })).toBeNull();
    expect(screen.getByRole("button", { name: "Familia PÉREZ TORRES" })).toBeInTheDocument();
  });

  it("names one family, counts several, and sends their ids as scope family", async () => {
    const user = userEvent.setup();
    render(<AnnouncementsCompose onBack={() => {}} families />);
    await user.click(await screen.findByRole("button", { name: "Familias" }));
    await user.click(await screen.findByRole("button", { name: "Familia GARCÍA TORRES" }));
    expect(screen.getByText("Familia GARCÍA TORRES", { selector: "strong" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Familia PÉREZ TORRES" }));
    expect(screen.getByText("2 familias", { selector: "strong" })).toBeInTheDocument();
    expect(within(pane()).getByText("2 familias")).toBeInTheDocument();

    await user.type(screen.getByLabelText("Título"), "Junta");
    await user.type(screen.getByLabelText("Mensaje"), "Jueves");
    await user.click(screen.getByRole("button", { name: /Enviar aviso/ }));
    await waitFor(() => expect(comms.announce).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({ scope: "family", targetIds: ["f1", "f2"], cycleId: "c2" })));
    expect(await screen.findByText("Enviado a 3 alumnos")).toBeInTheDocument();
  });

  it("stops at 50 families and says why", async () => {
    const user = userEvent.setup();
    families.listFamilies.mockResolvedValue(Array.from({ length: 51 }, (_, i) => family(`f${i}`, `FAMILIA ${String(i).padStart(2, "0")}`, [])));
    render(<AnnouncementsCompose onBack={() => {}} families />);
    await user.click(await screen.findByRole("button", { name: "Familias" }));
    await screen.findByRole("button", { name: "Familia FAMILIA 00" });
    for (let i = 0; i < 51; i += 1) await user.click(screen.getByRole("button", { name: `Familia FAMILIA ${String(i).padStart(2, "0")}` }));
    expect(screen.getByRole("button", { name: "Familia FAMILIA 50" })).toHaveAttribute("aria-pressed", "false");
    expect(within(pane()).getByText("50 familias")).toBeInTheDocument();
    expect(screen.getByText("Un aviso llega a 50 familias como máximo. Para más, usa Grupos, Niveles o Toda la escuela.")).toBeInTheDocument();
  });

  it("says so when the families cannot be read", async () => {
    const user = userEvent.setup();
    families.listFamilies.mockRejectedValue(new Error("down"));
    render(<AnnouncementsCompose onBack={() => {}} families />);
    await user.click(await screen.findByRole("button", { name: "Familias" }));
    expect(await screen.findByText("No pudimos leer las familias.")).toBeInTheDocument();
  });
});
