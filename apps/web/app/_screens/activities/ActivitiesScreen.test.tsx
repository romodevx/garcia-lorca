import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as api from "../../activities/api";
import * as roster from "../../_design/students/useStudentRoster";
import { ActivitiesScreen } from "./ActivitiesScreen";

vi.mock("../../activities/api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../../activities/api")>();
  return {
    ...real,
    listActivities: vi.fn(),
    createActivity: vi.fn(),
    updateActivity: vi.fn(),
    listEnrollments: vi.fn(),
    enrollStudents: vi.fn(),
    endEnrollments: vi.fn(),
    changeActivityOption: vi.fn(),
  };
});
vi.mock("../../_design/students/useStudentRoster", () => ({ useStudentRoster: vi.fn() }));
const m = vi.mocked(api);

const swim: api.Activity = {
  id: "a1", name: "Natación", enrolledCount: 12, periodicity: "monthly", active: true, lateFeeCents: 0,
  schedule: [{ weekday: 2, start: "15:00", end: "16:30" }, { weekday: 4, start: "15:00", end: "16:30" }],
  billingStartsOn: "2026-10-01", firstDueOn: "2026-10-10",
  options: [{ id: "o1", daysPerWeek: 1, amountCents: 60000 }, { id: "o2", daysPerWeek: null, amountCents: 90000 }],
};
const chess: api.Activity = { ...swim, id: "a2", name: "Ajedrez", enrolledCount: 0, active: false, periodicity: "per_cycle", options: [{ id: "o3", daysPerWeek: null, amountCents: 50000 }] };
const writer = ["activities:read", "activities:write"];

beforeEach(() => {
  m.listActivities.mockResolvedValue([swim, chess]);
  m.listEnrollments.mockResolvedValue([]);
  vi.mocked(roster.useStudentRoster).mockReturnValue({ status: "ok", students: [], catalog: { levels: [], grades: [], groups: [] } });
});
afterEach(() => vi.clearAllMocks());

function renderScreen(permissions: string[] = writer): void {
  render(<ActivitiesScreen onBack={() => undefined} permissions={permissions} today="2026-10-02" />);
}

describe("ActivitiesScreen (T17)", () => {
  it("lists the active activities with their week, periodicity, prices and pupils", async () => {
    renderScreen();
    const rows = await screen.findAllByTestId("activity-row");
    expect(rows).toHaveLength(1);
    expect(rows[0]).toHaveTextContent("Natación");
    expect(rows[0]).toHaveTextContent("Mar y Jue 15:00–16:30");
    expect(rows[0]).toHaveTextContent("Mensual");
    expect(rows[0]).toHaveTextContent("1 día $600.00 · Fija $900.00");
    expect(rows[0]).toHaveTextContent("12");
    expect(m.listActivities).toHaveBeenCalledWith(expect.any(String), true);
  });

  it("leads with how many are active and how many pupils are enrolled", async () => {
    renderScreen();
    expect(await screen.findByTestId("stat-active")).toHaveTextContent("1");
    expect(screen.getByTestId("stat-enrolled")).toHaveTextContent("12");
  });

  it("shows the inactive ones on their own tab", async () => {
    const user = userEvent.setup();
    renderScreen();
    await screen.findAllByTestId("activity-row");
    await user.click(screen.getByRole("tab", { name: /Inactivas/ }));
    const [row] = screen.getAllByTestId("activity-row");
    expect(row).toHaveTextContent("Ajedrez");
    expect(row).toHaveTextContent("Una vez por ciclo");
  });

  it("'Nueva actividad' opens the form page", async () => {
    const user = userEvent.setup();
    renderScreen();
    await screen.findAllByTestId("activity-row");
    await user.click(screen.getAllByRole("button", { name: /Nueva actividad/ })[0] as HTMLElement);
    expect(screen.getByRole("heading", { name: "Nueva actividad" })).toBeInTheDocument();
  });

  it("'Editar' opens the form with the activity", async () => {
    const user = userEvent.setup();
    renderScreen();
    const [row] = await screen.findAllByTestId("activity-row");
    await user.click(within(row as HTMLElement).getByRole("button", { name: /Editar/ }));
    expect(screen.getByRole("heading", { name: "Editar Natación" })).toBeInTheDocument();
    expect(screen.getByLabelText("Nombre")).toHaveValue("Natación");
  });

  it("'Inscribir alumnos' opens the enrolment page", async () => {
    const user = userEvent.setup();
    renderScreen();
    const [row] = await screen.findAllByTestId("activity-row");
    await user.click(within(row as HTMLElement).getByRole("button", { name: /Inscribir alumnos/ }));
    expect(await screen.findByRole("heading", { name: /Inscritos en Natación/ })).toBeInTheDocument();
  });

  it("without activities:write there is nothing to create or edit, only who is enrolled", async () => {
    renderScreen(["activities:read"]);
    const [row] = await screen.findAllByTestId("activity-row");
    expect(screen.queryByRole("button", { name: /Nueva actividad/ })).not.toBeInTheDocument();
    expect(within(row as HTMLElement).queryByRole("button", { name: /Editar/ })).not.toBeInTheDocument();
    expect(within(row as HTMLElement).getByRole("button", { name: /Ver inscritos/ })).toBeInTheDocument();
  });

  it("says why when the activities cannot be read", async () => {
    m.listActivities.mockRejectedValue(new Error("Módulo apagado"));
    renderScreen();
    expect(await screen.findByRole("alert")).toHaveTextContent("Módulo apagado");
  });

  it("says so when there are none yet", async () => {
    m.listActivities.mockResolvedValue([]);
    renderScreen();
    expect(await screen.findByText(/Todavía no hay actividades/)).toBeInTheDocument();
  });
});
