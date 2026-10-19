import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as commsApi from "../../comms/api";
import * as configApi from "../../config/api";
import * as studentsApi from "../../students/api";
import { API_URL } from "../../api-url";
import { VIEWS, type User } from "./views.testkit";

/*
 * Characterization of the Avisos screen (.gstack/redesign/avisos/behavior.md).
 * Written green against the screen as it was before the redesign; the new
 * layout must keep every one of these passing. Ids in the names (B2, D5…) point
 * at that inventory.
 */

vi.mock("../../comms/api", () => ({
  announce: vi.fn(),
  getBirthdayTemplate: vi.fn().mockResolvedValue({ title: "¡Feliz cumpleaños, {nombre}!", body: "Hola {nombre}", isDefault: true, updatedAt: "" }),
  updateBirthdayTemplate: vi.fn(),
}));
vi.mock("../../config/api", () => ({ listCycles: vi.fn(), listLevels: vi.fn(), listLabelledGroups: vi.fn() }));
vi.mock("../../students/api", () => ({ listStudentsWithRooms: vi.fn() }));

const comms = vi.mocked(commsApi);
const config = vi.mocked(configApi);
const students = vi.mocked(studentsApi);

const ACTIVE = { id: "c2", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" } as const;
const CLOSED = { id: "c1", name: "2025-2026", status: "closed", startsOn: "2025-08-01", endsOn: "2026-06-30" } as const;

beforeEach(() => {
  config.listCycles.mockResolvedValue([CLOSED, ACTIVE]);
  config.listLevels.mockResolvedValue([
    { id: "lv1", name: "Primaria", sepCode: "PRIM", enrollmentGranularity: "cycle" },
    { id: "lv2", name: "Secundaria", sepCode: "SEC", enrollmentGranularity: "cycle" },
  ]);
  config.listLabelledGroups.mockResolvedValue([
    { id: "g1", cycleId: "c2", gradeLevelId: "gl1", name: "A", capacity: 30, label: "1° A" },
    { id: "g2", cycleId: "c2", gradeLevelId: "gl1", name: "B", capacity: 30, label: "1° B" },
  ]);
  students.listStudentsWithRooms.mockResolvedValue([
    { id: "s1", curp: "C1", firstName: "Valentina", lastName: "Cruz", level: "primaria", levelId: "", gradeGroup: "1° A", guardianName: "Tutor", admissionStatus: "enrolled" },
    { id: "s2", curp: "C2", firstName: "Mateo", lastName: "Andrade", level: "primaria", levelId: "", gradeGroup: "1° B", guardianName: "Tutor", admissionStatus: "enrolled" },
  ]);
  comms.announce.mockResolvedValue({ created: 28, audience: 28 });
});
afterEach(() => vi.clearAllMocks());

async function write(user: User, title: string, body: string): Promise<void> {
  await user.type(screen.getByLabelText("Título"), title);
  await user.type(screen.getByLabelText("Mensaje"), body);
}

function sendButton(): HTMLElement {
  return screen.getByRole("button", { name: /Enviar aviso/ });
}

function lastSent(): commsApi.AnnounceInput | undefined {
  return comms.announce.mock.calls.at(-1)?.[1];
}

describe.each(VIEWS)("Avisos ($name) — behavior contract", ({ Screen, pickStudent }) => {
  it("A0: each list is read exactly once, and choosing an audience reads nothing more", async () => {
    const user = userEvent.setup();
    render(<Screen onBack={() => {}} />);
    await screen.findByRole("button", { name: "1° A" });
    await screen.findByDisplayValue("¡Feliz cumpleaños, {nombre}!");
    const expectLoadedOnce = (): void => {
      expect(config.listCycles).toHaveBeenCalledTimes(1);
      expect(config.listLevels).toHaveBeenCalledTimes(1);
      expect(students.listStudentsWithRooms).toHaveBeenCalledTimes(1);
      expect(comms.getBirthdayTemplate).toHaveBeenCalledTimes(1);
      expect(config.listLabelledGroups).toHaveBeenCalledTimes(1);
      expect(config.listLabelledGroups).toHaveBeenCalledWith(API_URL, "c2");
      expect(comms.announce).not.toHaveBeenCalled();
    };
    expectLoadedOnce();

    for (const name of ["Niveles", "Toda la escuela", "Grupos", "Un alumno"]) {
      await user.click(screen.getByRole("button", { name }));
    }
    await pickStudent(user, "Cruz Valentina");
    expectLoadedOnce();
  });

  it.each([
    { scope: "group", audience: "Grupos", target: "1° A", targetIds: ["g1"] },
    { scope: "level", audience: "Niveles", target: "Primaria", targetIds: ["lv1"] },
    { scope: "student", audience: "Un alumno", target: "Andrade Mateo", targetIds: ["s2"] },
  ])("A0: announce gets the whole payload for $audience", async ({ scope, audience, target, targetIds }) => {
    const user = userEvent.setup();
    render(<Screen onBack={() => {}} />);
    await screen.findByRole("button", { name: "1° A" });
    await user.click(screen.getByRole("button", { name: audience }));
    if (scope === "student") {
      await pickStudent(user, target);
    } else {
      await user.click(screen.getByRole("button", { name: target }));
    }
    await write(user, " Junta ", " Jueves ");
    await user.click(sendButton());
    await waitFor(() => expect(comms.announce).toHaveBeenCalledTimes(1));
    expect(comms.announce).toHaveBeenCalledWith(API_URL, {
      scope, targetIds, cycleId: "c2", title: "Junta", body: "Jueves", idempotencyKey: expect.any(String),
    });
  });

  it("A0: announce gets the whole payload for the whole school", async () => {
    const user = userEvent.setup();
    render(<Screen onBack={() => {}} />);
    await screen.findByRole("button", { name: "1° A" });
    await user.click(screen.getByRole("button", { name: "Toda la escuela" }));
    await write(user, "Junta", "Jueves");
    await user.click(sendButton());
    await waitFor(() => expect(comms.announce).toHaveBeenCalledTimes(1));
    expect(comms.announce).toHaveBeenCalledWith(API_URL, {
      scope: "cycle", cycleId: "c2", title: "Junta", body: "Jueves", idempotencyKey: expect.any(String),
    });
  });

  it("A1: says so when the cycles cannot be read", async () => {
    config.listCycles.mockRejectedValue(new Error("down"));
    render(<Screen onBack={() => {}} />);
    expect(await screen.findByText("No pudimos leer los ciclos.")).toBeInTheDocument();
  });

  it("A6: addresses the active cycle, not the first one listed", async () => {
    const user = userEvent.setup();
    render(<Screen onBack={() => {}} />);
    await user.click(await screen.findByRole("button", { name: "1° A" }));
    await write(user, "Junta", "Jueves");
    await user.click(sendButton());
    await waitFor(() => expect(comms.announce).toHaveBeenCalled());
    expect(lastSent()?.cycleId).toBe("c2");
    expect(config.listLabelledGroups).toHaveBeenCalledWith(expect.any(String), "c2");
  });

  it("B1: offers the four audiences and starts on groups", async () => {
    render(<Screen onBack={() => {}} />);
    expect(await screen.findByRole("button", { name: "1° A" })).toBeInTheDocument();
    for (const name of ["Un alumno", "Grupos", "Niveles", "Toda la escuela"]) {
      expect(screen.getByRole("button", { name })).toBeInTheDocument();
    }
  });

  it("B2: changing the audience forgets what was picked", async () => {
    const user = userEvent.setup();
    render(<Screen onBack={() => {}} />);
    await user.click(await screen.findByRole("button", { name: "1° A" }));
    await user.click(screen.getByRole("button", { name: "Niveles" }));
    await user.click(screen.getByRole("button", { name: "Grupos" }));
    expect(screen.getByRole("button", { name: "1° A" })).toHaveAttribute("aria-pressed", "false");
  });

  it("B4: a picked group can be unpicked", async () => {
    const user = userEvent.setup();
    render(<Screen onBack={() => {}} />);
    const group = await screen.findByRole("button", { name: "1° A" });
    await user.click(group);
    expect(group).toHaveAttribute("aria-pressed", "true");
    await user.click(group);
    expect(group).toHaveAttribute("aria-pressed", "false");
  });

  it("B5: picking a second pupil replaces the first", async () => {
    const user = userEvent.setup();
    render(<Screen onBack={() => {}} />);
    await screen.findByRole("button", { name: "1° A" });
    await user.click(screen.getByRole("button", { name: "Un alumno" }));
    await pickStudent(user, "Cruz Valentina");
    await pickStudent(user, "Andrade Mateo");
    expect(screen.getByRole("button", { name: "Andrade Mateo" })).toHaveAttribute("aria-pressed", "true");
    await write(user, "Recordatorio", "Traer uniforme");
    await user.click(sendButton());
    await waitFor(() => expect(comms.announce).toHaveBeenCalled());
    expect(lastSent()?.targetIds).toEqual(["s2"]);
  });

  it("B6: counts the picks once there is more than one", async () => {
    const user = userEvent.setup();
    render(<Screen onBack={() => {}} />);
    await user.click(await screen.findByRole("button", { name: "1° A" }));
    expect(screen.queryByText(/seleccionados/)).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "1° B" }));
    expect(screen.getByText(/2 seleccionados/)).toBeInTheDocument();
  });

  it("D1: refuses to send without a cycle", async () => {
    const user = userEvent.setup();
    config.listCycles.mockResolvedValue([]);
    render(<Screen onBack={() => {}} />);
    await screen.findByRole("button", { name: "Grupos" });
    await user.click(screen.getByRole("button", { name: "Toda la escuela" }));
    await write(user, "Junta", "Jueves");
    await user.click(sendButton());
    expect(await screen.findByRole("alert")).toHaveTextContent("No hay un ciclo activo al que dirigir el aviso.");
    expect(comms.announce).not.toHaveBeenCalled();
  });

  it("D3: asks for an audience when none was picked", async () => {
    const user = userEvent.setup();
    render(<Screen onBack={() => {}} />);
    await screen.findByRole("button", { name: "1° A" });
    await write(user, "Junta", "Jueves");
    await user.click(sendButton());
    expect(await screen.findByRole("alert")).toHaveTextContent("Elige a quién va dirigido.");
    expect(comms.announce).not.toHaveBeenCalled();
  });

  it("D4: the whole school goes without targets, trimmed", async () => {
    const user = userEvent.setup();
    render(<Screen onBack={() => {}} />);
    await screen.findByRole("button", { name: "1° A" });
    await user.click(screen.getByRole("button", { name: "Toda la escuela" }));
    await write(user, "  Junta  ", "  Jueves  ");
    await user.click(sendButton());
    await waitFor(() => expect(comms.announce).toHaveBeenCalled());
    const sent = lastSent();
    expect(sent?.scope).toBe("cycle");
    expect(sent).not.toHaveProperty("targetIds");
    expect(sent?.title).toBe("Junta");
    expect(sent?.body).toBe("Jueves");
  });

  it("D4: levels go by id", async () => {
    const user = userEvent.setup();
    render(<Screen onBack={() => {}} />);
    await screen.findByRole("button", { name: "1° A" });
    await user.click(screen.getByRole("button", { name: "Niveles" }));
    await user.click(screen.getByRole("button", { name: "Secundaria" }));
    await write(user, "Junta", "Jueves");
    await user.click(sendButton());
    await waitFor(() => expect(comms.announce).toHaveBeenCalled());
    expect(lastSent()?.scope).toBe("level");
    expect(lastSent()?.targetIds).toEqual(["lv2"]);
  });

  it("D4: Enter in Título sends the message", async () => {
    const user = userEvent.setup();
    render(<Screen onBack={() => {}} />);
    await user.click(await screen.findByRole("button", { name: "1° A" }));
    await user.type(screen.getByLabelText("Mensaje"), "Jueves");
    await user.type(screen.getByLabelText("Título"), "Junta{Enter}");
    await waitFor(() => expect(comms.announce).toHaveBeenCalledTimes(1));
    expect(lastSent()?.targetIds).toEqual(["g1"]);
  });

  it("D5: one pupil reads in the singular", async () => {
    const user = userEvent.setup();
    comms.announce.mockResolvedValue({ created: 1, audience: 1 });
    render(<Screen onBack={() => {}} />);
    await user.click(await screen.findByRole("button", { name: "1° A" }));
    await write(user, "Junta", "Jueves");
    await user.click(sendButton());
    expect(await screen.findByRole("status")).toHaveTextContent("Enviado a 1 alumno");
    expect(screen.getByRole("status")).not.toHaveTextContent("alumnos");
  });

  it("D5: says when the message had already gone out", async () => {
    const user = userEvent.setup();
    comms.announce.mockResolvedValue({ created: 0, audience: 28 });
    render(<Screen onBack={() => {}} />);
    await user.click(await screen.findByRole("button", { name: "1° A" }));
    await write(user, "Junta", "Jueves");
    await user.click(sendButton());
    expect(await screen.findByRole("status")).toHaveTextContent("ya se había enviado, no se repitió");
  });

  it("D5: a sent message clears the form and the next one gets a new key", async () => {
    const user = userEvent.setup();
    render(<Screen onBack={() => {}} />);
    await user.click(await screen.findByRole("button", { name: "1° A" }));
    await write(user, "Junta", "Jueves");
    await user.click(sendButton());
    await screen.findByRole("status");

    expect(screen.getByLabelText("Título")).toHaveValue("");
    expect(screen.getByLabelText("Mensaje")).toHaveValue("");
    expect(screen.getByRole("button", { name: "1° A" })).toHaveAttribute("aria-pressed", "false");
    const firstKey = lastSent()?.idempotencyKey;

    await user.click(screen.getByRole("button", { name: "1° B" }));
    await write(user, "Kermés", "Sábado");
    await user.click(sendButton());
    await waitFor(() => expect(comms.announce).toHaveBeenCalledTimes(2));
    expect(lastSent()?.idempotencyKey).not.toBe(firstKey);
  });

  it("D6: a retry after a failure keeps the same key", async () => {
    const user = userEvent.setup();
    comms.announce.mockRejectedValueOnce(new Error("se cayó la red"));
    render(<Screen onBack={() => {}} />);
    await user.click(await screen.findByRole("button", { name: "1° A" }));
    await write(user, "Junta", "Jueves");
    await user.click(sendButton());
    expect(await screen.findByRole("alert")).toHaveTextContent("se cayó la red");
    const firstKey = lastSent()?.idempotencyKey;

    await user.click(sendButton());
    await waitFor(() => expect(comms.announce).toHaveBeenCalledTimes(2));
    expect(lastSent()?.idempotencyKey).toBe(firstKey);
    expect(await screen.findByRole("status")).toHaveTextContent("28 alumnos");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("F1: Volver al inicio goes back", async () => {
    const user = userEvent.setup();
    const onBack = vi.fn();
    render(<Screen onBack={onBack} />);
    await user.click(await screen.findByRole("button", { name: /Volver al inicio/ }));
    expect(onBack).toHaveBeenCalledTimes(1);
  });
});
