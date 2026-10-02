import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as commsApi from "../../comms/api";
import * as configApi from "../../config/api";
import * as studentsApi from "../../students/api";
import { AnnouncementsScreen } from "./AnnouncementsScreen";

// The screens read labelled groups (ISSUE-002); label them from the listGroups mock.
beforeEach(() => {
  vi.mocked(configApi.listLabelledGroups).mockImplementation(async (...args) =>
    (await vi.mocked(configApi.listGroups)(...args)).map((g) => ({ ...g, label: g.name })));
});


vi.mock("../../comms/api", () => ({
  announce: vi.fn(),
  getBirthdayTemplate: vi.fn().mockResolvedValue({ title: "¡Feliz cumpleaños, {nombre}!", body: "Hola {nombre}", isDefault: true, updatedAt: "" }),
  updateBirthdayTemplate: vi.fn(),
}));
vi.mock("../../config/api", () => ({ listCycles: vi.fn(), listLevels: vi.fn(), listGroups: vi.fn(), listLabelledGroups: vi.fn() }));
vi.mock("../../students/api", () => ({ listStudents: vi.fn() }));

const comms = vi.mocked(commsApi);
const config = vi.mocked(configApi);
const students = vi.mocked(studentsApi);

beforeEach(() => {
  config.listCycles.mockResolvedValue([{ id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" }]);
  config.listLevels.mockResolvedValue([{ id: "lv1", name: "Primaria", sepCode: "PRIM", enrollmentGranularity: "cycle" }]);
  config.listGroups.mockResolvedValue([{ id: "g1", cycleId: "c1", gradeLevelId: "gl1", name: "1° A", capacity: 30 }]);
  students.listStudents.mockResolvedValue([
    { id: "s1", curp: "C1", firstName: "Valentina", lastName: "Cruz", level: "primaria", levelId: "", gradeGroup: "1° A", guardianName: "Tutor", admissionStatus: "enrolled" },
  ]);
  comms.announce.mockResolvedValue({ created: 28, audience: 28 });
});
afterEach(() => vi.clearAllMocks());

describe("AnnouncementsScreen", () => {
  // T11: the school writes its birthday greeting where it writes to families.
  it("offers the birthday greeting below the announcement", async () => {
    render(<AnnouncementsScreen onBack={() => {}} />);
    expect(await screen.findByDisplayValue("¡Feliz cumpleaños, {nombre}!")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Felicitación de cumpleaños" })).toBeInTheDocument();
  });

  it("sends one message to a chosen group", async () => {
    const user = userEvent.setup();
    render(<AnnouncementsScreen onBack={() => {}} />);

    await user.click(await screen.findByRole("button", { name: "1° A" }));
    await user.type(screen.getByLabelText("Título"), "Junta de padres");
    await user.type(screen.getByLabelText("Mensaje"), "Jueves 8:00");
    await user.click(screen.getByRole("button", { name: /Enviar aviso/ }));

    await waitFor(() => expect(comms.announce).toHaveBeenCalled());
    const sent = comms.announce.mock.calls[0]?.[1];
    expect(sent?.scope).toBe("group");
    expect(sent?.targetIds).toEqual(["g1"]);
    expect(sent?.idempotencyKey).toBeTruthy();
    expect(await screen.findByRole("status")).toHaveTextContent("28 alumnos");
  });

  // Whole-school needs no target, and asking for one would be a field with
  // nothing in it that still blocks the send.
  it("does not ask for a target when the audience is the whole school", async () => {
    const user = userEvent.setup();
    render(<AnnouncementsScreen onBack={() => {}} />);
    await screen.findByRole("button", { name: "1° A" });

    await user.click(screen.getByRole("button", { name: "Toda la escuela" }));

    expect(screen.queryByRole("button", { name: "1° A" })).not.toBeInTheDocument();
  });

  it("refuses to send a message with nothing in it", async () => {
    const user = userEvent.setup();
    render(<AnnouncementsScreen onBack={() => {}} />);
    await screen.findByRole("button", { name: "1° A" });

    await user.click(screen.getByRole("button", { name: /Enviar aviso/ }));

    expect(comms.announce).not.toHaveBeenCalled();
    expect(await screen.findByRole("alert")).toBeInTheDocument();
  });

  // The office has to read the refusal, not a generic failure: "nobody matched"
  // is the difference between fixing the audience and believing it went out.
  it("shows the gateway's own words when nobody matched", async () => {
    const user = userEvent.setup();
    comms.announce.mockRejectedValue(new Error("nadie coincide con ese destino"));
    render(<AnnouncementsScreen onBack={() => {}} />);

    await user.click(await screen.findByRole("button", { name: "1° A" }));
    await user.type(screen.getByLabelText("Título"), "Junta");
    await user.type(screen.getByLabelText("Mensaje"), "Jueves");
    await user.click(screen.getByRole("button", { name: /Enviar aviso/ }));

    expect(await screen.findByRole("alert")).toHaveTextContent("nadie coincide con ese destino");
  });

  // "1° A y 1° B" is one message the school writes once, not two it writes twice
  // and sends at different times.
  it("sends to several groups at once", async () => {
    const user = userEvent.setup();
    config.listGroups.mockResolvedValue([
      { id: "g1", cycleId: "c1", gradeLevelId: "gl1", name: "1° A", capacity: 30 },
      { id: "g2", cycleId: "c1", gradeLevelId: "gl1", name: "1° B", capacity: 30 },
    ]);
    render(<AnnouncementsScreen onBack={() => {}} />);

    await user.click(await screen.findByRole("button", { name: "1° A" }));
    await user.click(screen.getByRole("button", { name: "1° B" }));
    await user.type(screen.getByLabelText("Título"), "Junta");
    await user.type(screen.getByLabelText("Mensaje"), "Jueves");
    await user.click(screen.getByRole("button", { name: /Enviar aviso/ }));

    await waitFor(() => expect(comms.announce).toHaveBeenCalled());
    expect(comms.announce.mock.calls[0]?.[1]?.targetIds).toEqual(["g1", "g2"]);
  });

  // The note to one family. Picking a second pupil replaces the first: a note
  // is not a circular, and letting it become one silently would be worse.
  it("sends to one pupil, one at a time", async () => {
    const user = userEvent.setup();
    render(<AnnouncementsScreen onBack={() => {}} />);
    await screen.findByRole("button", { name: "1° A" });

    await user.click(screen.getByRole("button", { name: "Un alumno" }));
    await user.click(await screen.findByRole("button", { name: "Cruz Valentina" }));
    await user.type(screen.getByLabelText("Título"), "Recordatorio");
    await user.type(screen.getByLabelText("Mensaje"), "Traer uniforme");
    await user.click(screen.getByRole("button", { name: /Enviar aviso/ }));

    await waitFor(() => expect(comms.announce).toHaveBeenCalled());
    const sent = comms.announce.mock.calls[0]?.[1];
    expect(sent?.scope).toBe("student");
    expect(sent?.targetIds).toEqual(["s1"]);
  });
});
