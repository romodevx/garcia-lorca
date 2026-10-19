import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { MODULES, USERS, isModuleVisible } from "../_design/data";
import { ALL_MODULES_ON, HOME_VARIANTS, renderHome } from "../home/testing/renderHome";
import { listCycles } from "../config/api";
import { listCharges } from "../finance/api";
import { Dashboard } from "./Dashboard";

// The dashboard now reads real KPIs from the gateway — mock the clients so the
// component renders without hitting the network.
vi.mock("../students/api", () => ({ listStudents: vi.fn().mockResolvedValue([]) }));
vi.mock("../config/api", () => ({ listCycles: vi.fn().mockResolvedValue([]) }));
vi.mock("../finance/api", () => ({ listCharges: vi.fn().mockResolvedValue([]) }));

// The full owner permission set: READ on every backend-backed module. The
// modules outside this set (scholarships/uniforms/billing/reports) carry a permission
// the owner does not hold here, so they never appear.
const OWNER_PERMS = [
  "students:read",
  "finance:read",
  "attendance:read",
  "grades:read",
  "comms:read",
  "config:read",
  "users:read",
] as const;

const NOW = new Date("2026-06-12T09:00:00");
const visibleFor = (perms: readonly string[]) => MODULES.filter((m) => isModuleVisible(m, perms));

describe.each(HOME_VARIANTS)("Dashboard (%s home)", (_variant, refresh) => {
  const render = (ui: Parameters<typeof renderHome>[0]) => renderHome(ui, refresh);

  it("renders a launcher card for every module the owner can read", () => {
    render(<Dashboard user={USERS.admin} onOpen={() => {}} permissions={OWNER_PERMS} now={NOW} />);

    const expected = visibleFor(OWNER_PERMS);
    for (const m of expected) {
      expect(screen.getByText(m.label)).toBeInTheDocument();
    }
    expect(expected.map((m) => screen.getByText(m.label))).toHaveLength(expected.length);
  });

  // C2-013: with finance OFF the home showed Finanzas, Egresos, Actividades and
  // Becas as live cards that opened a placeholder. A module that is off is not there.
  it("draws no card for a module whose flag is off, nor for one that lives in it", () => {
    const owner = [...OWNER_PERMS, "expenses:read", "activities:read", "scholarships:read"];
    renderHome(<Dashboard user={USERS.admin} onOpen={() => {}} permissions={owner} now={NOW} />, refresh, {
      ...ALL_MODULES_ON,
      module_finance: false,
    });

    for (const label of ["Finanzas", "Registro de pagos", "Egresos", "Actividades", "Becas"]) {
      expect(screen.queryByText(label)).not.toBeInTheDocument();
    }
    expect(screen.getByText("Estudiantes")).toBeInTheDocument();
  });

  // C2-013: nor a money KPI, nor the ledger read behind it (it would only 404).
  it("neither shows nor reads the money KPIs when finance is off", async () => {
    vi.mocked(listCharges).mockClear();
    vi.mocked(listCycles).mockResolvedValue([{ id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-24", endsOn: "2027-07-09" }]);
    renderHome(<Dashboard user={USERS.admin} onOpen={() => {}} permissions={OWNER_PERMS} now={NOW} />, refresh, {
      ...ALL_MODULES_ON,
      module_finance: false,
    });

    await waitFor(() => expect(listCycles).toHaveBeenCalled());
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(screen.queryByText("Por cobrar")).toBeNull();
    expect(listCharges).not.toHaveBeenCalled();
  });

  it("hides modules the owner has no backend permission for (Becas/Inventario/Facturación/Reportes)", () => {
    render(<Dashboard user={USERS.admin} onOpen={() => {}} permissions={OWNER_PERMS} now={NOW} />);

    for (const label of ["Becas", "Inventario", "Facturación", "Reportes"]) {
      expect(screen.queryByText(label)).not.toBeInTheDocument();
    }
  });

  it("scopes the grid to the caller's permissions (grades+attendance only)", () => {
    render(
      <Dashboard user={USERS.teacher} onOpen={() => {}} permissions={["grades:read", "attendance:read"]} now={NOW} />
    );

    expect(screen.getByText("Calificaciones")).toBeInTheDocument();
    expect(screen.getByText("Asistencia")).toBeInTheDocument();
    // Modules the teacher cannot read must NOT appear.
    expect(screen.queryByText("Finanzas y Adeudos")).not.toBeInTheDocument();
    expect(screen.queryByText("Administración")).not.toBeInTheDocument();
    expect(screen.queryByText("Estudiantes")).not.toBeInTheDocument();
  });

  // The maestro role (iam 0022) holds exactly these four: Calificaciones and
  // Asistencia for the teacher's own classes, and nothing else.
  // T16: plus Horarios — a teacher reads their own week through teaching:read.
  it("a teacher (maestro role) sees exactly Asistencia, Calificaciones and Horarios", () => {
    const maestro = ["teaching:read", "teaching:write", "attendance_own:read", "attendance_own:write"];
    render(<Dashboard user={USERS.teacher} onOpen={() => {}} permissions={maestro} now={NOW} />);

    expect(visibleFor(maestro).map((m) => m.label).sort()).toEqual(["Asistencia", "Calificaciones", "Horarios"]);
    for (const m of MODULES) {
      if (m.id === "grades" || m.id === "attendance" || m.id === "schedule") expect(screen.getByText(m.label)).toBeInTheDocument();
      else expect(screen.queryByText(m.label)).not.toBeInTheDocument();
    }
  });

  it("gates the KPI shortcuts by permission (grades-only user sees no finance/students KPIs)", () => {
    render(<Dashboard user={USERS.teacher} onOpen={() => {}} permissions={["grades:read"]} now={NOW} />);

    expect(screen.queryByRole("button", { name: /Por cobrar/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Cobrado/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Alumnos/ })).not.toBeInTheDocument();
  });

  it("shows a friendly empty state when the caller has zero matching modules", () => {
    render(<Dashboard user={USERS.teacher} onOpen={() => {}} permissions={[]} now={NOW} />);

    expect(screen.getByText(/No tienes módulos asignados/)).toBeInTheDocument();
    // No module tiles at all.
    expect(screen.queryByText("Calificaciones")).not.toBeInTheDocument();
  });

  it("shows a loading skeleton (not all modules) before permissions resolve", () => {
    // permissions === undefined ⇒ me() has not resolved yet. The grid must not
    // flash every module and then hide them; it shows a skeleton instead.
    render(<Dashboard user={USERS.admin} onOpen={() => {}} now={NOW} />);

    expect(screen.getByRole("status", { name: /Cargando módulos/ })).toBeInTheDocument();
    // Neither real tiles nor the empty state while loading.
    expect(screen.queryByText("Calificaciones")).not.toBeInTheDocument();
    expect(screen.queryByText(/No tienes módulos asignados/)).not.toBeInTheDocument();
  });

  it("does not render modules deactivated from the panel (e.g. Documentos)", () => {
    render(<Dashboard user={USERS.admin} onOpen={() => {}} permissions={OWNER_PERMS} now={NOW} />);

    const hidden = MODULES.filter((m) => m.hidden);
    expect(hidden.some((m) => m.id === "documents")).toBe(true);
    for (const m of hidden) {
      expect(screen.queryByText(m.label)).not.toBeInTheDocument();
      expect(screen.queryByText(m.desc)).not.toBeInTheDocument();
    }
  });

  it("greets by time of day with the real user", () => {
    render(<Dashboard user={USERS.admin} onOpen={() => {}} permissions={OWNER_PERMS} now={NOW} />);
    expect(screen.getByRole("heading", { name: /Buenos días, Director/ })).toBeInTheDocument();
  });

  it("navigates from a KPI card", async () => {
    const user = userEvent.setup();
    const onOpen = vi.fn();
    render(<Dashboard user={USERS.admin} onOpen={onOpen} permissions={OWNER_PERMS} now={NOW} />);

    await user.click(screen.getByRole("button", { name: /Por cobrar/ }));
    expect(onOpen).toHaveBeenCalledWith("finance");
  });
});

describe.each(HOME_VARIANTS)("Dashboard when the permissions could not be loaded (%s home)", (_variant, refresh) => {
  const render = (ui: Parameters<typeof renderHome>[0]) => renderHome(ui, refresh);

  // The regression this exists for (dev, 2026-09-07): the gateway answered 403
  // to /me, AppShell caught it as "non-fatal", and permissions stayed undefined
  // — which rendered the LOADING SKELETON forever. The owner saw a panel that
  // looked like it was still thinking and reported "no veo ningún módulo".
  //
  // A failure that renders as loading is worse than an error: it tells the
  // person to wait for something that is never coming.
  it("says so, instead of pretending it is still loading", () => {
    render(<Dashboard user={USERS.admin} onOpen={() => {}} permissionsFailed now={NOW} />);

    expect(screen.getByRole("alert")).toHaveTextContent(/permisos/i);
    expect(screen.queryByTestId("modules-skeleton")).not.toBeInTheDocument();
  });

  it("offers a retry, so the way out is not logging out", async () => {
    const onRetry = vi.fn();
    const user = userEvent.setup();
    render(<Dashboard user={USERS.admin} onOpen={() => {}} permissionsFailed onRetryPermissions={onRetry} now={NOW} />);

    await user.click(screen.getByRole("button", { name: /reintentar/i }));
    expect(onRetry).toHaveBeenCalled();
  });

  // And the ordinary loading state is untouched: undefined without a failure is
  // still "we are asking", which is the honest reading of it.
  it("still shows the skeleton while the answer is genuinely pending", () => {
    render(<Dashboard user={USERS.admin} onOpen={() => {}} now={NOW} />);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});

// The ONE switch between the two homes (PLAN-redesign-home §3.3, E-flag).
describe("Dashboard picks its home from module_ui_refresh", () => {
  it("OFF: today's grid under the \"Módulos\" heading", () => {
    renderHome(<Dashboard user={USERS.admin} onOpen={() => {}} permissions={OWNER_PERMS} now={NOW} />, false);
    expect(screen.getByRole("heading", { level: 2, name: "Módulos" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Académico" })).not.toBeInTheDocument();
  });

  it("OFF: no \"Módulos\" heading over nothing — a role with no module sees only the notice (CC-315)", () => {
    renderHome(<Dashboard user={USERS.admin} onOpen={() => {}} permissions={[]} now={NOW} />, false);
    expect(screen.queryByRole("heading", { name: "Módulos" })).not.toBeInTheDocument();
    expect(screen.queryByText("Todo el sistema, a la vista")).not.toBeInTheDocument();
  });

  it("ON: the areas, and no \"Módulos\" heading", () => {
    renderHome(<Dashboard user={USERS.admin} onOpen={() => {}} permissions={OWNER_PERMS} now={NOW} />, true);
    expect(screen.getByRole("region", { name: "Académico" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Módulos" })).not.toBeInTheDocument();
    expect(screen.queryByText("Todo el sistema, a la vista")).not.toBeInTheDocument();
  });
});
