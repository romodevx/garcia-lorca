// Regression: ISSUE-002 — the home screen read the students list, the cycles and
// the ledger for everybody, so a maestro (and a cajera) opened the app to three
// 403s in the console and a "Ciclo —" under the greeting.
// Found by /qa on 2026-09-25
// Report: .gstack/qa-reports/qa-report-localhost-3002-2026-09-25-t2.md
import { screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { USERS } from "../_design/data";
import { listCycles } from "../config/api";
import { listCharges } from "../finance/api";
import { listStudents } from "../students/api";
import { HOME_VARIANTS, renderHome } from "../home/testing/renderHome";
import { Dashboard } from "./Dashboard";

vi.mock("../students/api", () => ({ listStudents: vi.fn().mockResolvedValue([]) }));
vi.mock("../config/api", () => ({ listCycles: vi.fn().mockResolvedValue([{ id: "c1", name: "2026-2027", status: "active" }]) }));
vi.mock("../finance/api", () => ({ listCharges: vi.fn().mockResolvedValue([]) }));

const NOW = new Date("2026-06-12T09:00:00");
const MAESTRO = ["teaching:read", "teaching:write", "attendance_own:read", "attendance_own:write"];

beforeEach(() => {
  vi.mocked(listStudents).mockClear();
  vi.mocked(listCycles).mockClear();
  vi.mocked(listCharges).mockClear();
});

describe.each(HOME_VARIANTS)("Dashboard — reads only what the caller may read (%s home)", (_variant, refresh) => {
  const render = (ui: Parameters<typeof renderHome>[0]) => renderHome(ui, refresh);

  it("a maestro triggers no students, cycles or ledger read", async () => {
    render(<Dashboard user={USERS.teacher} onOpen={() => {}} permissions={MAESTRO} now={NOW} />);

    await screen.findByText("Calificaciones");
    expect(listStudents).not.toHaveBeenCalled();
    expect(listCycles).not.toHaveBeenCalled();
    expect(listCharges).not.toHaveBeenCalled();
  });

  it("does not print a placeholder cycle when it cannot know it", async () => {
    render(<Dashboard user={USERS.teacher} onOpen={() => {}} permissions={MAESTRO} now={NOW} />);

    await screen.findByText("Calificaciones");
    expect(screen.queryByText(/Ciclo —/)).toBeNull();
  });

  it("reads nothing before the permissions resolve", () => {
    render(<Dashboard user={USERS.teacher} onOpen={() => {}} permissions={undefined} now={NOW} />);

    expect(listStudents).not.toHaveBeenCalled();
    expect(listCycles).not.toHaveBeenCalled();
  });

  it("the owner still gets every KPI and the cycle name", async () => {
    render(
      <Dashboard user={USERS.admin} onOpen={() => {}} permissions={["students:read", "finance:read", "config:read"]} now={NOW} />,
    );

    await waitFor(() => expect(listCharges).toHaveBeenCalledWith(expect.anything(), { cycleId: "c1" }));
    expect(listStudents).toHaveBeenCalledTimes(1);
    expect((await screen.findAllByText(/Ciclo 2026-2027/)).length).toBeGreaterThan(0);
  });
});
