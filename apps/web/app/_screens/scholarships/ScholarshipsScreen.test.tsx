import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as api from "../../scholarships/api";
import * as roster from "../../_design/students/useStudentRoster";
import { ScholarshipsScreen } from "./ScholarshipsScreen";

vi.mock("../../scholarships/api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../../scholarships/api")>();
  return {
    ...real,
    listScholarships: vi.fn(),
    listScholarshipTargets: vi.fn(),
    createScholarship: vi.fn(),
    updateScholarship: vi.fn(),
    listScholarshipHolders: vi.fn(),
    assignScholarship: vi.fn(),
    unassignScholarship: vi.fn(),
  };
});
vi.mock("../../_design/students/useStudentRoster", () => ({ useStudentRoster: vi.fn() }));
const m = vi.mocked(api);

const ACTIVITY = "activity:a1000000-0000-0000-0000-000000000001";
const hermanos: api.Scholarship = { id: "b1", name: "HERMANOS", kind: "percent", value: 5000, targets: ["tuition", "enrollment"], active: true, assignedCount: 14 };
const convenio: api.Scholarship = { id: "b2", name: "CONVENIO", kind: "fixed_price", value: 100000, targets: [ACTIVITY], active: true, assignedCount: 3 };
const vieja: api.Scholarship = { id: "b3", name: "DEPORTIVA", kind: "fixed_off", value: 50000, targets: ["tuition"], active: false, assignedCount: 0 };
const targets: api.ScholarshipTarget[] = [
  { key: "enrollment", label: "Inscripción", group: "enrollment" },
  { key: "tuition", label: "Colegiatura", group: "tuition" },
  { key: ACTIVITY, label: "Natación", group: "activity" },
];
const writer = ["scholarships:read", "scholarships:write"];

beforeEach(() => {
  m.listScholarships.mockResolvedValue([hermanos, convenio, vieja]);
  m.listScholarshipTargets.mockResolvedValue(targets);
  m.listScholarshipHolders.mockResolvedValue([]);
  vi.mocked(roster.useStudentRoster).mockReturnValue({ status: "ok", students: [], catalog: { levels: [], grades: [], groups: [] } });
});
afterEach(() => vi.clearAllMocks());

function renderScreen(permissions: string[] = writer): void {
  render(<ScholarshipsScreen onBack={() => undefined} permissions={permissions} />);
}

describe("ScholarshipsScreen (T18)", () => {
  it("lists the active scholarships with their value, what they apply to and how many hold them", async () => {
    renderScreen();
    const rows = await screen.findAllByTestId("scholarship-row");
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("HERMANOS");
    expect(rows[0]).toHaveTextContent("Porcentaje");
    expect(rows[0]).toHaveTextContent("50 %");
    expect(rows[0]).toHaveTextContent("Inscripción, Colegiatura");
    expect(rows[0]).toHaveTextContent("14");
    expect(rows[1]).toHaveTextContent("Paga $1,000.00");
    expect(rows[1]).toHaveTextContent("Natación");
    expect(m.listScholarships).toHaveBeenCalledWith(expect.any(String), true);
  });

  it("leads with how many are active and how many pupils hold one", async () => {
    renderScreen();
    expect(await screen.findByTestId("stat-active")).toHaveTextContent("2");
    expect(screen.getByTestId("stat-holders")).toHaveTextContent("17");
    expect(screen.getByTestId("stat-inactive")).toHaveTextContent("1");
  });

  it("shows the inactive ones on their own tab", async () => {
    const user = userEvent.setup();
    renderScreen();
    await screen.findAllByTestId("scholarship-row");
    await user.click(screen.getByRole("tab", { name: /Inactivas/ }));
    const [row] = screen.getAllByTestId("scholarship-row");
    expect(row).toHaveTextContent("DEPORTIVA");
    expect(row).toHaveTextContent("$500.00 de descuento");
  });

  it("'Nueva beca' opens the drawer with the targets", async () => {
    const user = userEvent.setup();
    renderScreen();
    await screen.findAllByTestId("scholarship-row");
    await user.click(screen.getAllByRole("button", { name: /Nueva beca/ })[0] as HTMLElement);
    const dialog = await screen.findByRole("dialog", { name: "Nueva beca" });
    expect(within(dialog).getByRole("checkbox", { name: "Natación" })).toBeInTheDocument();
  });

  it("saving reloads the list and says so", async () => {
    const user = userEvent.setup();
    m.updateScholarship.mockResolvedValue(hermanos);
    renderScreen();
    const [row] = await screen.findAllByTestId("scholarship-row");
    await user.click(within(row as HTMLElement).getByRole("button", { name: /Editar/ }));
    await user.click(within(await screen.findByRole("dialog")).getByRole("button", { name: /Guardar beca/ }));
    expect(await screen.findByText("Beca HERMANOS guardada")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(m.listScholarships).toHaveBeenCalledTimes(2);
  });

  it("'Asignar alumnos' opens the assignment page", async () => {
    const user = userEvent.setup();
    renderScreen();
    const [row] = await screen.findAllByTestId("scholarship-row");
    await user.click(within(row as HTMLElement).getByRole("button", { name: /Asignar alumnos/ }));
    expect(await screen.findByRole("heading", { name: "Alumnos con Beca HERMANOS" })).toBeInTheDocument();
  });

  it("without scholarships:write there is nothing to create or edit, only who holds each", async () => {
    renderScreen(["scholarships:read"]);
    const [row] = await screen.findAllByTestId("scholarship-row");
    expect(screen.queryByRole("button", { name: /Nueva beca/ })).not.toBeInTheDocument();
    expect(within(row as HTMLElement).queryByRole("button", { name: /Editar/ })).not.toBeInTheDocument();
    expect(within(row as HTMLElement).getByRole("button", { name: /Ver alumnos/ })).toBeInTheDocument();
  });

  it("says why when the scholarships cannot be read", async () => {
    m.listScholarships.mockRejectedValue(new Error("Módulo apagado"));
    renderScreen();
    expect(await screen.findByRole("alert")).toHaveTextContent("Módulo apagado");
  });

  it("says so when there are none yet", async () => {
    m.listScholarships.mockResolvedValue([]);
    renderScreen();
    expect(await screen.findByText(/Todavía no hay becas/)).toBeInTheDocument();
  });
});
