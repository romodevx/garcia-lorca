import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AttendanceSheet } from "../../attendance/sheet";
import * as teachingApi from "../../teaching/api";
import type { AttendanceClass } from "../../teaching/api";
import { TeacherAttendance } from "./TeacherAttendance";

// The teacher surface talks to the gateway EXCLUSIVELY through the self-scoped
// teaching client — never the admin attendance/config/students clients (the
// teacher lacks those permissions). The transport is covered by
// app/teaching/api.test.ts; the list itself by PassSheet.test.tsx.
vi.mock("../../teaching/api", () => ({
  listAttendanceClasses: vi.fn(),
  getTeachingAttendanceSheet: vi.fn(),
  saveTeachingAttendance: vi.fn(),
}));

const teaching = vi.mocked(teachingApi);
const TODAY = "2026-09-01";

const daily = (groupId: string, groupLabel: string): AttendanceClass => ({ groupId, groupLabel, levelName: "Primaria", mode: "daily" });
const perClass = (groupId: string, groupLabel: string, subjectId: string, subjectName: string): AttendanceClass => ({
  groupId,
  groupLabel,
  levelName: "Secundaria",
  mode: "per_class",
  subjectId,
  subjectName,
  assignmentId: `${subjectId}-${groupId}`,
});

const sheet: AttendanceSheet = {
  mode: "daily",
  date: TODAY,
  editable: true,
  windowDays: 7,
  session: null,
  rows: [{ enrollmentId: "en1", studentId: "st1", name: "Ada Lovelace", status: "present", note: "" }],
};

const renderScreen = (onBack: () => void = () => {}) => render(<TeacherAttendance onBack={onBack} today={TODAY} />);

beforeEach(() => {
  // The picker keeps its place in the URL: every test starts at the first step.
  window.history.replaceState(null, "", "/asistencia");
  teaching.listAttendanceClasses.mockResolvedValue([daily("g1", "1° PRIMARIA · A")]);
  teaching.getTeachingAttendanceSheet.mockResolvedValue(sheet);
  teaching.saveTeachingAttendance.mockResolvedValue(undefined);
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

describe("TeacherAttendance — daily groups only", () => {
  it("one group: today's list opens with no step, asking the server for the day list", async () => {
    renderScreen();

    await screen.findByRole("button", { name: "Ada Lovelace: Presente" });
    expect(teaching.getTeachingAttendanceSheet).toHaveBeenCalledWith({ groupId: "g1", date: TODAY });
    expect(screen.queryByRole("heading", { name: "Elige el grupo" })).toBeNull();
    expect(screen.getByRole("navigation", { name: "Ruta" })).toHaveTextContent("Asistencia›1° PRIMARIA · A");
  });

  it("two groups: the group step only, kept in the URL", async () => {
    const user = userEvent.setup();
    teaching.listAttendanceClasses.mockResolvedValue([daily("g1", "1° PRIMARIA · A"), daily("g2", "2° PRIMARIA · B")]);
    renderScreen();

    expect(await screen.findByRole("heading", { name: "Elige el grupo" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Elige la asignatura" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "2° PRIMARIA · B" }));

    await waitFor(() => expect(teaching.getTeachingAttendanceSheet).toHaveBeenCalledWith({ groupId: "g2", date: TODAY }));
    expect(window.location.search).toBe("?grupo=g2");
  });
});

describe("TeacherAttendance — per-class lists", () => {
  it("subject first, then the list of that subject's only group, sending the subject", async () => {
    const user = userEvent.setup();
    teaching.listAttendanceClasses.mockResolvedValue([
      perClass("gA", "1° SECUNDARIA · A", "s-mat", "Matemáticas"),
      perClass("gA", "1° SECUNDARIA · A", "s-esp", "Español"),
    ]);
    renderScreen();

    const subjects = await screen.findByRole("list", { name: "Asignaturas" });
    await user.click(within(subjects).getByRole("button", { name: /Matemáticas/ }));

    await waitFor(() =>
      expect(teaching.getTeachingAttendanceSheet).toHaveBeenCalledWith({ groupId: "gA", date: TODAY, subjectId: "s-mat" }),
    );
    // A skipped step (one group) stays out of the URL, like every picker step.
    expect(window.location.search).toBe("?asignatura=s-mat");
    expect(await screen.findByText("Pase de lista · 1° SECUNDARIA · A · Matemáticas")).toBeInTheDocument();
  });

  it("a single per-class item skips every step", async () => {
    teaching.listAttendanceClasses.mockResolvedValue([perClass("gA", "1° SECUNDARIA · A", "s-mat", "Matemáticas")]);
    renderScreen();

    await screen.findByRole("button", { name: "Ada Lovelace: Presente" });
    expect(teaching.getTeachingAttendanceSheet).toHaveBeenCalledWith({ groupId: "gA", date: TODAY, subjectId: "s-mat" });
  });

  it("mixed: 'Lista del día' sits beside the subjects and leads to the daily groups, with no subject sent", async () => {
    const user = userEvent.setup();
    teaching.listAttendanceClasses.mockResolvedValue([
      daily("g1", "1° PRIMARIA · A"),
      daily("g2", "2° PRIMARIA · B"),
      perClass("gA", "1° SECUNDARIA · A", "s-mat", "Matemáticas"),
    ]);
    renderScreen();

    const subjects = await screen.findByRole("list", { name: "Asignaturas" });
    expect(within(subjects).getByRole("button", { name: /Matemáticas/ })).toBeInTheDocument();
    await user.click(within(subjects).getByRole("button", { name: /Lista del día/ }));

    const groups = await screen.findByRole("list", { name: "Grupos" });
    expect(within(groups).getAllByRole("button")).toHaveLength(2);
    await user.click(within(groups).getByRole("button", { name: "1° PRIMARIA · A" }));

    await waitFor(() => expect(teaching.getTeachingAttendanceSheet).toHaveBeenCalledWith({ groupId: "g1", date: TODAY }));
    expect(window.location.search).toBe("?asignatura=dia&grupo=g1");
  });
});

describe("TeacherAttendance — unsaved changes", () => {
  beforeEach(() => {
    teaching.listAttendanceClasses.mockResolvedValue([daily("g1", "1° PRIMARIA · A"), daily("g2", "2° PRIMARIA · B")]);
    window.history.replaceState(null, "", "/asistencia?grupo=g1");
  });

  it("switching group with unsaved marks asks first, and staying keeps the list", async () => {
    const user = userEvent.setup();
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    renderScreen();

    await user.click(await screen.findByRole("button", { name: "Ada Lovelace: Presente" }));
    await user.click(screen.getByRole("button", { name: "Cambiar" }));
    await user.click(screen.getByRole("button", { name: "2° PRIMARIA · B" }));

    expect(confirm).toHaveBeenCalledTimes(1);
    expect(window.location.search).toBe("?grupo=g1");
    expect(screen.getByRole("button", { name: "Ada Lovelace: Ausente" })).toBeInTheDocument();
  });

  it("does not ask when nothing changed", async () => {
    const user = userEvent.setup();
    const confirm = vi.spyOn(window, "confirm");
    const onBack = vi.fn();
    renderScreen(onBack);

    await screen.findByRole("button", { name: "Ada Lovelace: Presente" });
    await user.click(screen.getByRole("button", { name: "Volver al inicio" }));

    expect(confirm).not.toHaveBeenCalled();
    expect(onBack).toHaveBeenCalled();
  });

  it("going back home with unsaved marks asks first", async () => {
    const user = userEvent.setup();
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    const onBack = vi.fn();
    renderScreen(onBack);

    await user.click(await screen.findByRole("button", { name: "Ada Lovelace: Presente" }));
    await user.click(screen.getByRole("button", { name: "Volver al inicio" }));

    expect(confirm).toHaveBeenCalled();
    expect(onBack).not.toHaveBeenCalled();
  });
});

describe("TeacherAttendance — states", () => {
  it("shows an error when the classes fail to load", async () => {
    teaching.listAttendanceClasses.mockRejectedValue(new Error("boom"));
    renderScreen();
    expect(await screen.findByRole("alert")).toHaveTextContent(/No se pudieron cargar/);
  });

  it("tells a teacher without groups who to ask", async () => {
    teaching.listAttendanceClasses.mockResolvedValue([]);
    renderScreen();
    expect(await screen.findByText(/Aún no tienes grupos asignados\. Pide a la dirección/)).toBeInTheDocument();
  });
});
