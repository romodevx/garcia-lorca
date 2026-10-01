import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import * as configApi from "../../config/api";
import { StudentsScreen } from "./StudentsScreen";

vi.mock("../../students/api", async (importOriginal) => ({
  ...(await importOriginal<typeof studentsApi>()),
  listStudents: vi.fn(),
  listEnrollments: vi.fn(),
  listNotes: vi.fn(),
  addNote: vi.fn(),
  editNote: vi.fn(),
}));
vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  listGroups: vi.fn(),
  listLabelledGroups: vi.fn(),
  listLevels: vi.fn(),
  listGradeLevels: vi.fn(),
}));

const students = vi.mocked(studentsApi);
const config = vi.mocked(configApi);

const valentina: studentsApi.Student = {
  id: "st-1", curp: "CURP1", firstName: "Valentina", lastName: "Cruz", level: "primaria", levelId: "",
  gradeGroup: "1° A", guardianName: "Ana Tutor", admissionStatus: "enrolled",
};
const note: studentsApi.StudentNote = {
  id: "n-1", studentId: "st-1", body: "Alérgica a la nuez", authorUserId: "u-me", authorName: "Gabriela Rojas",
  createdAt: "2026-09-24T16:00:00Z", voided: false,
};

beforeEach(() => {
  window.history.replaceState(null, "", "/alumnos");
  students.listStudents.mockResolvedValue([valentina]);
  students.listEnrollments.mockResolvedValue([]);
  students.listNotes.mockResolvedValue([note]);
  config.listCycles.mockResolvedValue([]);
  config.listGroups.mockResolvedValue([]);
  config.listLabelledGroups.mockResolvedValue([]);
  config.listLevels.mockResolvedValue([]);
  config.listGradeLevels.mockResolvedValue([]);
});
afterEach(() => vi.clearAllMocks());

// T6: the notes open from the list, without entering the pupil's edit page.
describe("StudentsScreen · notas", () => {
  it("opens a pupil's notes from the row, in a dialog, and gives the focus back on close", async () => {
    const user = userEvent.setup();
    const onEdit = vi.fn();
    render(<StudentsScreen onBack={() => {}} onEdit={onEdit} userId="u-me" canWriteNotes />);
    const trigger = await screen.findByRole("button", { name: "Notas de Cruz Valentina" });
    await user.click(trigger);

    const dialog = await screen.findByRole("dialog", { name: "Notas · Cruz Valentina" });
    expect(await within(dialog).findByText("Alérgica a la nuez")).toBeInTheDocument();
    expect(students.listNotes).toHaveBeenCalledWith(expect.any(String), "st-1");
    expect(within(dialog).getByLabelText("Nueva nota")).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Editar nota" })).toBeInTheDocument();
    expect(onEdit).not.toHaveBeenCalled();

    await user.click(within(dialog).getByRole("button", { name: "Cerrar" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  it("adds a note from the list's dialog", async () => {
    const user = userEvent.setup();
    students.addNote.mockResolvedValue({ ...note, id: "n-2", body: "Recoge la abuela" });
    render(<StudentsScreen onBack={() => {}} userId="u-me" canWriteNotes />);
    await user.click(await screen.findByRole("button", { name: "Notas de Cruz Valentina" }));
    const dialog = await screen.findByRole("dialog", { name: "Notas · Cruz Valentina" });
    await user.type(within(dialog).getByLabelText("Nueva nota"), "Recoge la abuela");
    await user.click(within(dialog).getByRole("button", { name: "Agregar nota" }));
    await waitFor(() => expect(students.addNote).toHaveBeenCalledWith(expect.any(String), "st-1", "Recoge la abuela"));
    expect(await within(dialog).findByText("Recoge la abuela")).toBeInTheDocument();
  });

  it("only shows the notes to whoever may not write them", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} userId="u-me" />);
    await user.click(await screen.findByRole("button", { name: "Notas de Cruz Valentina" }));
    const dialog = await screen.findByRole("dialog", { name: "Notas · Cruz Valentina" });
    expect(await within(dialog).findByText("Alérgica a la nuez")).toBeInTheDocument();
    expect(within(dialog).queryByLabelText("Nueva nota")).not.toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: "Editar nota" })).not.toBeInTheDocument();
  });
});
