import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AttendanceReport, AttendanceReportQuery, ReportFormat } from "../../attendance/report";
import type { FilterCatalog } from "../../_design/students/studentFilter";
import { AttendanceReportPanel } from "./AttendanceReportPanel";

const TODAY = "2026-09-17";

const catalog: FilterCatalog = {
  levels: [{ id: "lv-pri", name: "PRIMARIA" }, { id: "lv-sec", name: "SECUNDARIA" }],
  grades: [
    { id: "gr-1s", levelId: "lv-sec", name: "1°", ordinal: 1 },
    { id: "gr-2s", levelId: "lv-sec", name: "2°", ordinal: 2 },
    { id: "gr-1p", levelId: "lv-pri", name: "1°", ordinal: 1 },
  ],
  groups: [
    { id: "g-1sa", gradeLevelId: "gr-1s", name: "A" },
    { id: "g-1sb", gradeLevelId: "gr-1s", name: "B" },
  ],
};

const report: AttendanceReport = {
  groupId: "g-1sa",
  groupLabel: "1° SECUNDARIA · A",
  from: "2026-09-01",
  to: TODAY,
  listsTaken: 12,
  rows: [
    { enrollmentId: "e1", studentId: "s1", name: "ÁLVAREZ PAZ ANA", active: true, present: 8, absent: 1, late: 3, excused: 0 },
    { enrollmentId: "e2", studentId: "s2", name: "ZAMORA RUIZ ZOE", active: false, present: 12, absent: 0, late: 0, excused: 0 },
  ],
  totals: { present: 20, absent: 1, late: 3, excused: 0 },
};

const load = vi.fn<(q: AttendanceReportQuery) => Promise<AttendanceReport>>();
const download = vi.fn<(q: AttendanceReportQuery, format: ReportFormat) => Promise<{ blob: Blob; filename: string }>>();

function renderPanel() {
  return render(<AttendanceReportPanel catalog={catalog} today={TODAY} load={load} download={download} />);
}

async function chooseGroup(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.click(within(screen.getByRole("group", { name: "Nivel" })).getByRole("button", { name: "SECUNDARIA" }));
  await user.click(within(screen.getByRole("group", { name: "Grado" })).getByRole("button", { name: "1°" }));
  await user.click(within(screen.getByRole("group", { name: "Grupo" })).getByRole("button", { name: "A" }));
}

beforeEach(() => {
  load.mockResolvedValue(report);
  download.mockResolvedValue({ blob: new Blob(["%PDF"]), filename: "reporte.pdf" });
  vi.stubGlobal("URL", Object.assign(URL, { createObjectURL: vi.fn(() => "blob:x"), revokeObjectURL: vi.fn() }));
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe("AttendanceReportPanel (T12)", () => {
  it("asks for a group first and reads nothing until one is chosen", () => {
    renderPanel();
    expect(screen.getByText("Elige nivel, grado y grupo para ver su reporte.")).toBeInTheDocument();
    expect(load).not.toHaveBeenCalled();
  });

  it("chains level → grade → group: a level shows only its grades, a grade only its groups", async () => {
    const user = userEvent.setup();
    renderPanel();
    expect(screen.queryByRole("group", { name: "Grado" })).toBeNull();
    await user.click(within(screen.getByRole("group", { name: "Nivel" })).getByRole("button", { name: "SECUNDARIA" }));
    const grades = within(screen.getByRole("group", { name: "Grado" })).getAllByRole("button").map((b) => b.textContent);
    expect(grades).toEqual(["1°", "2°"]);
  });

  it("loads the chosen group from the first of the month to today and shows each pupil's counts with tardies", async () => {
    const user = userEvent.setup();
    renderPanel();
    await chooseGroup(user);

    await waitFor(() => expect(load).toHaveBeenCalledWith({ groupId: "g-1sa", from: "2026-09-01", to: TODAY }));
    const table = await screen.findByRole("table", { name: "Asistencia por alumno" });
    const headers = within(table).getAllByRole("columnheader").map((h) => h.textContent);
    expect(headers).toEqual(["Alumno", "Presentes", "Ausentes", "Retardos", "Justificadas"]);
    const ana = within(table).getByRole("row", { name: /ÁLVAREZ PAZ ANA/ });
    expect(within(ana).getAllByRole("cell").map((c) => c.textContent)).toEqual(["ÁLVAREZ PAZ ANA", "8", "1", "3", "0"]);
    const total = within(table).getByRole("row", { name: /Total/ });
    expect(within(total).getAllByRole("cell").map((c) => c.textContent)).toEqual(["Total", "20", "1", "3", "0"]);
    expect(screen.getByText("12 listas tomadas")).toBeInTheDocument();
    expect(within(table).getByRole("row", { name: /ZAMORA RUIZ ZOE \(baja\)/ })).toBeInTheDocument();
  });

  it("reloads when the range changes", async () => {
    const user = userEvent.setup();
    renderPanel();
    await chooseGroup(user);
    await screen.findByRole("table", { name: "Asistencia por alumno" });

    const from = screen.getByLabelText("Desde");
    await user.clear(from);
    await user.type(from, "2026-08-15");

    await waitFor(() => expect(load).toHaveBeenLastCalledWith({ groupId: "g-1sa", from: "2026-08-15", to: TODAY }));
  });

  it("exports the same report to PDF and Excel", async () => {
    const user = userEvent.setup();
    renderPanel();
    await chooseGroup(user);
    await screen.findByRole("table", { name: "Asistencia por alumno" });

    await user.click(screen.getByRole("button", { name: "PDF" }));
    await waitFor(() => expect(download).toHaveBeenCalledWith({ groupId: "g-1sa", from: "2026-09-01", to: TODAY }, "pdf"));
    await user.click(screen.getByRole("button", { name: "Excel" }));
    await waitFor(() => expect(download).toHaveBeenCalledWith({ groupId: "g-1sa", from: "2026-09-01", to: TODAY }, "xlsx"));
  });

  it("shows the gateway's refusal", async () => {
    const user = userEvent.setup();
    load.mockRejectedValue(new Error("El reporte abarca como máximo 366 días."));
    renderPanel();
    await chooseGroup(user);
    expect(await screen.findByRole("alert")).toHaveTextContent("El reporte abarca como máximo 366 días.");
  });
});
