import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../config/api";
import * as familiesApi from "../../families/api";
import type { FamiliesAccess } from "../../families/model";
import * as studentsApi from "../../students/api";
import { StudentsScreen } from "./StudentsScreen";

// ADR 0041 D1: Familias is a second view of Alumnos, not a module of the menu;
// the directory gets a "Familia" column and a "Sin familia" chip.

vi.mock("../../students/api", async (importOriginal) => ({
  ...(await importOriginal<typeof studentsApi>()),
  listStudents: vi.fn(),
  listEnrollments: vi.fn(),
}));
vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  listLabelledGroups: vi.fn(),
  listLevels: vi.fn(),
  listGradeLevels: vi.fn(),
}));
vi.mock("../../families/api", async (importOriginal) => ({
  ...(await importOriginal<typeof familiesApi>()),
  listFamilies: vi.fn(),
  listFamilySuggestions: vi.fn(),
  getFamily: vi.fn(),
  listFamilyContacts: vi.fn(),
}));
const students = vi.mocked(studentsApi);
const config = vi.mocked(configApi);
const families = vi.mocked(familiesApi);

const OWNER: FamiliesAccess = { canReadGuardians: true, canWriteGuardians: true, canUpdateStudents: true };
const pupil = (id: string, lastName: string, firstName: string, familyId?: string): studentsApi.Student => ({
  id, curp: `CURP${id}`, firstName, lastName, maternalLastName: "LÓPEZ", level: "Primaria", levelId: "pri", gradeGroup: "A", guardianName: "", admissionStatus: "enrolled", ...(familyId === undefined ? {} : { familyId }),
});

beforeEach(() => {
  window.history.replaceState(null, "", "/alumnos");
  students.listStudents.mockResolvedValue([pupil("s1", "GARCÍA", "ANA", "f1"), pupil("s2", "GARCÍA", "LUIS", "f1"), pupil("s3", "SOTO", "IAN")]);
  students.listEnrollments.mockResolvedValue([]);
  config.listCycles.mockResolvedValue([]);
  config.listLevels.mockResolvedValue([]);
  config.listGradeLevels.mockResolvedValue([]);
  config.listLabelledGroups.mockResolvedValue([]);
  families.listFamilies.mockResolvedValue([{ id: "f1", name: "GARCÍA LÓPEZ", studentIds: ["s1", "s2"], accountCount: 0, createdAt: "" }]);
  families.listFamilySuggestions.mockResolvedValue([]);
  families.getFamily.mockResolvedValue({ id: "f1", name: "GARCÍA LÓPEZ", studentIds: ["s1"], createdAt: "", members: [] });
  families.listFamilyContacts.mockResolvedValue([]);
});
afterEach(() => vi.clearAllMocks());

const rows = (): string[] =>
  screen.queryAllByRole("button", { name: /^Ver ficha de / }).map((b) => (b.getAttribute("aria-label") ?? "").replace("Ver ficha de ", ""));

describe("StudentsScreen — Familias off", () => {
  it("shows nothing of families and never asks for them", async () => {
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("GARCÍA LÓPEZ ANA");
    expect(screen.queryByRole("button", { name: "Familias" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Sin familia/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Familia / })).toBeNull();
    expect(families.listFamilies).not.toHaveBeenCalled();
    expect(families.listFamilySuggestions).not.toHaveBeenCalled();
  });
});

describe("StudentsScreen — Familias on", () => {
  it("switches between Alumnos and Familias and keeps it in the address", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} families={OWNER} />);
    await screen.findByText("GARCÍA LÓPEZ ANA");
    expect(screen.getByRole("button", { name: "Alumnos", pressed: true })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Familias" }));
    expect(await screen.findByRole("heading", { name: "Todas las familias" })).toBeInTheDocument();
    expect(window.location.search).toContain("vista=familias");
    expect(rows()).toEqual([]);

    await user.click(screen.getByRole("button", { name: "Alumnos" }));
    await waitFor(() => expect(rows()).toHaveLength(3));
    expect(window.location.search).not.toContain("vista");
  });

  it("opens on Familias when the address says so", async () => {
    window.history.replaceState(null, "", "/alumnos?vista=familias");
    render(<StudentsScreen onBack={() => {}} families={OWNER} />);
    expect(await screen.findByRole("heading", { name: "Todas las familias" })).toBeInTheDocument();
  });

  it("opens a family from the address (the pupil's file sends here)", async () => {
    window.history.replaceState(null, "", "/alumnos?vista=familias&familia=f1");
    render(<StudentsScreen onBack={() => {}} families={OWNER} />);
    expect(await screen.findByRole("dialog", { name: "Familia GARCÍA LÓPEZ" })).toBeInTheDocument();
  });

  it("names each pupil's family in the directory; it opens the family", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} families={OWNER} />);
    const links = await screen.findAllByRole("button", { name: "Familia GARCÍA LÓPEZ" });
    expect(links).toHaveLength(2);
    await user.click(links[0] as HTMLElement);
    expect(await screen.findByRole("dialog", { name: "Familia GARCÍA LÓPEZ" })).toBeInTheDocument();
    expect(window.location.search).toContain("familia=f1");
  });

  it("'Sin familia' keeps only the pupils without one", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} families={OWNER} />);
    await screen.findAllByRole("button", { name: "Familia GARCÍA LÓPEZ" });
    await user.click(screen.getByRole("button", { name: /Sin familia/ }));
    expect(rows()).toEqual(["SOTO LÓPEZ IAN"]);
    expect(window.location.search).toContain("sinfamilia=1");
  });

  it("'Alumnos sin familia' on the Familias view lands on that filter", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} families={OWNER} />);
    await user.click(await screen.findByRole("button", { name: "Familias" }));
    await user.click(await screen.findByTestId("families-stat-orphans"));
    await waitFor(() => expect(rows()).toEqual(["SOTO LÓPEZ IAN"]));
    expect(screen.getByRole("button", { name: /Sin familia/, pressed: true })).toBeInTheDocument();
  });

  it("a family's child opens their ficha from the drawer", async () => {
    const user = userEvent.setup();
    families.getFamily.mockResolvedValue({
      id: "f1", name: "GARCÍA LÓPEZ", studentIds: ["s1"], createdAt: "",
      members: [{ id: "s1", firstName: "ANA", lastName: "GARCÍA", maternalLastName: "LÓPEZ", level: "Primaria", gradeGroup: "A", admissionStatus: "enrolled", draft: false }],
    });
    render(<StudentsScreen onBack={() => {}} families={OWNER} />);
    await user.click((await screen.findAllByRole("button", { name: "Familia GARCÍA LÓPEZ" }))[0] as HTMLElement);
    const dialog = await screen.findByRole("dialog", { name: "Familia GARCÍA LÓPEZ" });
    await user.click(await within(dialog).findByRole("button", { name: "GARCÍA LÓPEZ ANA" }));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Familia GARCÍA LÓPEZ" })).toBeNull());
    expect(screen.getAllByText("CURPs1").length).toBeGreaterThan(0);
  });
});
