import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../config/api";
import * as teachersApi from "../../teachers/api";
import { TeachersScreen } from "./TeachersScreen";

// The screens read labelled groups (ISSUE-002); label them from the listGroups mock.
beforeEach(() => {
  vi.mocked(configApi.listLabelledGroups).mockImplementation(async (...args) =>
    (await vi.mocked(configApi.listGroups)(...args)).map((g) => ({ ...g, label: g.name })));
});


/**
 * Pide el CAMPO y no su ayuda: el (?) de junto es un botón que se llama
 * "Ayuda sobre <campo>", así que una consulta laxa encuentra los dos.
 */
const CAMPO = { selector: "input, select, textarea" } as const;

vi.mock("../../teachers/api", async (importOriginal) => ({
  ...(await importOriginal<typeof teachersApi>()),
  listTeachers: vi.fn(),
  createTeacher: vi.fn(),
  listTeachingAssignments: vi.fn(),
  createTeachingAssignment: vi.fn(),
  grantTeacherAccess: vi.fn(),
}));
vi.mock("../../config/api", async (importOriginal) => ({
  ...(await importOriginal<typeof configApi>()),
  listCycles: vi.fn(),
  listGroups: vi.fn(), listLabelledGroups: vi.fn(),
  listSubjects: vi.fn(),
}));

const teachers = vi.mocked(teachersApi);
const config = vi.mocked(configApi);

const ada = { id: "t1", firstName: "Ada", lastName: "Lovelace", email: "ada@escuela.mx", userId: "" };

beforeEach(() => {
  teachers.listTeachers.mockResolvedValue([ada]);
  teachers.listTeachingAssignments.mockResolvedValue([]);
  config.listCycles.mockResolvedValue([
    { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" },
  ]);
  config.listGroups.mockResolvedValue([]);
  config.listSubjects.mockResolvedValue([]);
});
afterEach(() => vi.clearAllMocks());

describe("the Docentes module", () => {
  // Regression: a principal with teachers:read only could not even LOOK at a
  // teacher's load. Everyone who sees the roster gets the button; the page is
  // read-only without teachers:write (TeacherLoadScreen's own test).
  // Found by /code-review on 2026-09-25
  it("offers Carga académica to everyone who can see the roster, read-only principals included", async () => {
    const onOpenLoad = vi.fn();
    render(<TeachersScreen onBack={() => {}} onOpenLoad={onOpenLoad} />);

    await userEvent.setup().click(await screen.findByRole("button", { name: "Carga académica de Ada Lovelace" }));

    expect(onOpenLoad).toHaveBeenCalledWith("t1");
  });

  it("opens on the staff roster", async () => {
    render(<TeachersScreen onBack={() => {}} onOpenLoad={() => {}} />);

    expect(await screen.findByRole("heading", { name: "Docentes", level: 1 })).toBeInTheDocument();
    // Scoped to the roster list: the assignment panel offers every teacher as a
    // dropdown option too, so the name appears twice on a working screen.
    const roster = await screen.findByRole("list");
    expect(within(roster).getByText(/Ada Lovelace/)).toBeInTheDocument();
  });

  it("adds a teacher to the roster", async () => {
    const user = userEvent.setup();
    teachers.createTeacher.mockResolvedValue({
      id: "t2", firstName: "Grace", lastName: "Hopper", email: "grace@escuela.mx", userId: "",
    });
    render(<TeachersScreen onBack={() => {}} onOpenLoad={() => {}} />);
    const form = await screen.findByRole("form", { name: "Agregar profesor" });

    await user.type(within(form).getByLabelText(/Nombre/, CAMPO), "Grace");
    await user.type(within(form).getByLabelText(/Apellidos/, CAMPO), "Hopper");
    await user.type(within(form).getByLabelText(/Correo/, CAMPO), "grace@escuela.mx");
    await user.click(within(form).getByRole("button", { name: "Crear profesor" }));

    await waitFor(() => expect(teachers.createTeacher).toHaveBeenCalled());
    const roster = await screen.findByRole("list");
    await waitFor(() => expect(within(roster).getByText(/Grace Hopper/)).toBeInTheDocument());
  });

  it("says so, instead of showing empty dropdowns, when the catalogue is closed to this account", async () => {
    // Assigning a teacher needs groups and subjects, which belong to
    // Configuración. A teachers:read holder without config:read gets a 403 there
    // — and an empty dropdown would read as "the school has no groups".
    config.listCycles.mockRejectedValue(new configApi.ConfigError(403, "forbidden"));
    render(<TeachersScreen onBack={() => {}} onOpenLoad={() => {}} />);

    expect(await screen.findByText(/no tienes acceso al catálogo/i)).toBeInTheDocument();
    // The roster itself still works: it does not depend on the catalogue.
    expect(await screen.findByText(/Ada Lovelace/)).toBeInTheDocument();
  });

  // Regression: the carga académica page reads cycles, levels and grades from
  // Configuración, so without config:read the whole page errored. Without the
  // catalogue it could only show bare ids, so the roster does not offer it.
  // Found by /code-review on 2026-09-25
  it("does not offer Carga académica when the catalogue is closed, and says why", async () => {
    config.listCycles.mockRejectedValue(new configApi.ConfigError(403, "forbidden"));
    render(<TeachersScreen onBack={() => {}} onOpenLoad={() => {}} />);

    expect(
      await screen.findByText(
        "No tienes acceso al catálogo académico; pide a la dirección el permiso de Configuración para ver nombres y asignar materias.",
      ),
    ).toBeInTheDocument();
    expect(await screen.findByText(/Ada Lovelace/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Carga académica/ })).not.toBeInTheDocument();
  });
});

describe("the Docentes module — acceso al sistema", () => {
  it("with the permission, giving access links the teacher and counts it", async () => {
    const user = userEvent.setup();
    teachers.grantTeacherAccess.mockResolvedValue({
      teacher: { ...ada, userId: "u1" }, userId: "u1", email: ada.email, status: "invited",
    });
    render(<TeachersScreen onBack={() => {}} onOpenLoad={vi.fn()} canGrantAccess />);

    await user.click(await screen.findByRole("button", { name: "Dar acceso al sistema a Ada Lovelace" }));

    await waitFor(() => expect(teachers.grantTeacherAccess).toHaveBeenCalledWith(expect.any(String), "t1"));
    const stat = screen.getByText("Con acceso al sistema").parentElement as HTMLElement;
    await waitFor(() => expect(stat).toHaveTextContent("1"));
  });

  it("without it, the button is not offered", async () => {
    render(<TeachersScreen onBack={() => {}} onOpenLoad={vi.fn()} />);

    await screen.findByRole("button", { name: "Carga académica de Ada Lovelace" });
    expect(screen.queryByRole("button", { name: /Dar acceso al sistema/ })).toBeNull();
  });
});
