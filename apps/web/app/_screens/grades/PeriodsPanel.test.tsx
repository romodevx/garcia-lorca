import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../config/api";
import { GradeRequestError } from "../../grades/monthly";
import * as periodsApi from "../../grades/periods";
import type { GradingPeriod, PeriodCaptureProgress, PeriodsClient } from "../../grades/periods";
import { PeriodsPanel } from "./PeriodsPanel";

vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  listGradeLevels: vi.fn(),
  listLabelledGroups: vi.fn(),
}));
vi.mock(import("../../grades/periods"), async (original) => ({ ...(await original()), periodsClient: vi.fn() }));

const config = vi.mocked(configApi);
const periods = vi.mocked(periodsApi);

const OPEN: GradingPeriod = {
  termId: "t1", termName: "1.er trimestre", ordinal: 1, levelId: "l1", levelName: "PRIMARIA",
  months: ["2026-09", "2026-10", "2026-11"], status: "open", version: 0, progress: { captured: 412, expected: 450 },
};
const CLOSED: GradingPeriod = {
  termId: "t2", termName: "2.º trimestre", ordinal: 2, levelId: "l1", levelName: "PRIMARIA",
  months: ["2026-12", "2027-01", "2027-02"], status: "closed", closedAt: "2026-12-01T18:00:00Z", closedByName: "Luis Beltrán",
  version: 2, progress: { captured: 450, expected: 450 },
};
const SECUNDARIA: GradingPeriod = { ...OPEN, termId: "t4", levelId: "l2", levelName: "SECUNDARIA", progress: { captured: 0, expected: 90 } };

const MISSING: PeriodCaptureProgress = {
  termId: "t1", months: OPEN.months, expected: 450, captured: 412,
  missing: [
    { groupId: "g1", groupLabel: "1° PRIMARIA · A", subjectName: "Matemáticas", teacherName: "Ada Lovelace", missingCount: 30 },
    { groupId: "g1", groupLabel: "1° PRIMARIA · A", subjectName: "Historia", teacherName: "Alan Turing", missingCount: 5 },
    { groupId: "g2", groupLabel: "2° PRIMARIA · A", subjectName: "Español", teacherName: "Grace Hopper", missingCount: 3 },
  ],
};
const COMPLETE: PeriodCaptureProgress = { ...MISSING, captured: 450, missing: [] };

type MockClient = { [K in keyof PeriodsClient]: ReturnType<typeof vi.fn> };
let client: MockClient;

beforeEach(() => {
  client = {
    list: vi.fn().mockResolvedValue([OPEN, CLOSED, SECUNDARIA]),
    progress: vi.fn().mockResolvedValue(MISSING),
    close: vi.fn().mockResolvedValue({ status: "closed", version: 1, reportCards: 124 }),
    reopen: vi.fn().mockResolvedValue({ status: "open" }),
    reportCards: vi.fn().mockResolvedValue([]),
    reportCardPdf: vi.fn().mockResolvedValue(new Blob(["%PDF"], { type: "application/pdf" })),
  };
  periods.periodsClient.mockReturnValue(client as unknown as PeriodsClient);
  config.listCycles.mockResolvedValue([
    { id: "c0", name: "2025-2026", status: "closed", startsOn: "2025-08-01", endsOn: "2026-07-31" },
    { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-07-31" },
  ]);
  config.listGradeLevels.mockResolvedValue([{ id: "p1", levelId: "l1", name: "1° PRIMARIA", ordinal: 1 }]);
  config.listLabelledGroups.mockResolvedValue([
    { id: "g1", cycleId: "c1", gradeLevelId: "p1", name: "A", capacity: 30, label: "1° PRIMARIA · A" },
    { id: "gs", cycleId: "c1", gradeLevelId: "s1", name: "A", capacity: 30, label: "1° SECUNDARIA · A" },
  ]);
});

afterEach(() => {
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});

const card = async (name: string): Promise<HTMLElement> => screen.findByRole("article", { name });

describe("PeriodsPanel — cards", () => {
  it("shows the active cycle's periods per level, with status, months and capture progress", async () => {
    render(<PeriodsPanel canClose />);

    const open = await card("PRIMARIA · 1.er trimestre");
    expect(client.list).toHaveBeenCalledWith("c1");
    expect(within(open).getByText("Abierto")).toBeInTheDocument();
    expect(within(open).getByText("Sep · Oct · Nov")).toBeInTheDocument();
    expect(within(open).getByText("412 de 450 capturadas")).toBeInTheDocument();
    expect(within(open).getByRole("progressbar")).toHaveAttribute("aria-valuenow", "412");
    expect(screen.getByRole("heading", { name: "SECUNDARIA" })).toBeInTheDocument();
  });

  it("a closed period says its version, when and by whom, and offers Reabrir and Ver boletas", async () => {
    render(<PeriodsPanel canClose />);

    const closed = await card("PRIMARIA · 2.º trimestre");
    expect(within(closed).getByText("Cerrado v2")).toBeInTheDocument();
    expect(within(closed).getByText(/1 dic.* 2026 · por Luis Beltrán/)).toBeInTheDocument();
    expect(within(closed).getByRole("button", { name: "Reabrir" })).toBeInTheDocument();
    expect(within(closed).getByRole("button", { name: "Ver boletas" })).toBeInTheDocument();
    expect(within(closed).queryByRole("button", { name: "Revisar y cerrar" })).toBeNull();
  });

  it("without grades:close there is nothing to close or reopen", async () => {
    render(<PeriodsPanel canClose={false} />);

    const open = await card("PRIMARIA · 1.er trimestre");
    expect(within(open).queryByRole("button", { name: "Revisar y cerrar" })).toBeNull();
    expect(within(await card("PRIMARIA · 2.º trimestre")).queryByRole("button", { name: "Reabrir" })).toBeNull();
    expect(within(await card("PRIMARIA · 2.º trimestre")).getByRole("button", { name: "Ver boletas" })).toBeInTheDocument();
  });

  it("says so when the list cannot load", async () => {
    client.list.mockRejectedValue(new Error("boom"));
    render(<PeriodsPanel canClose />);
    expect(await screen.findByRole("alert")).toHaveTextContent("No se pudieron cargar los periodos.");
  });
});

describe("PeriodsPanel — Revisar y cerrar", () => {
  it("lists what is missing by group and only offers closing anyway, which publishes the boletas", async () => {
    const user = userEvent.setup();
    render(<PeriodsPanel canClose />);
    await user.click(within(await card("PRIMARIA · 1.er trimestre")).getByRole("button", { name: "Revisar y cerrar" }));

    const dialog = await screen.findByRole("dialog", { name: "Cerrar PRIMARIA · 1.er trimestre" });
    expect(client.progress).toHaveBeenCalledWith("t1");
    expect(await within(dialog).findByText("412 de 450 capturadas")).toBeInTheDocument();
    const groupA = within(dialog).getByRole("group", { name: "1° PRIMARIA · A" });
    expect(within(groupA).getByText("Matemáticas · Ada Lovelace · faltan 30")).toBeInTheDocument();
    expect(within(groupA).getByText("Historia · Alan Turing · faltan 5")).toBeInTheDocument();
    expect(within(dialog).getByRole("group", { name: "2° PRIMARIA · A" })).toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: "Cerrar periodo" })).toBeNull();

    await user.click(within(dialog).getByRole("button", { name: "Cerrar de todos modos" }));

    expect(client.close).toHaveBeenCalledWith("t1", true);
    expect(await screen.findByRole("status")).toHaveTextContent("Se publicaron 124 boletas");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(client.list).toHaveBeenCalledTimes(2);
  });

  it("with nothing missing it closes normally", async () => {
    const user = userEvent.setup();
    client.progress.mockResolvedValue(COMPLETE);
    render(<PeriodsPanel canClose />);
    await user.click(within(await card("PRIMARIA · 1.er trimestre")).getByRole("button", { name: "Revisar y cerrar" }));

    const dialog = await screen.findByRole("dialog");
    await user.click(await within(dialog).findByRole("button", { name: "Cerrar periodo" }));

    expect(within(dialog).queryByRole("button", { name: "Cerrar de todos modos" })).toBeNull();
    expect(client.close).toHaveBeenCalledWith("t1", false);
    expect(await screen.findByRole("status")).toHaveTextContent("Se publicaron 124 boletas");
  });

  it("a MISSING_GRADES refusal (a capture was deleted meanwhile) shows the server's words and the missing list again", async () => {
    const user = userEvent.setup();
    client.progress.mockResolvedValueOnce(COMPLETE).mockResolvedValueOnce(MISSING);
    client.close.mockRejectedValueOnce(new GradeRequestError(409, "Faltan 38 calificaciones por capturar", "MISSING_GRADES"));
    render(<PeriodsPanel canClose />);
    await user.click(within(await card("PRIMARIA · 1.er trimestre")).getByRole("button", { name: "Revisar y cerrar" }));

    const dialog = await screen.findByRole("dialog");
    await user.click(await within(dialog).findByRole("button", { name: "Cerrar periodo" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent("Faltan 38 calificaciones por capturar");
    expect(await within(dialog).findByRole("button", { name: "Cerrar de todos modos" })).toBeInTheDocument();
    expect(client.progress).toHaveBeenCalledTimes(2);
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("Cancelar closes the dialog and gives the focus back to its button", async () => {
    const user = userEvent.setup();
    render(<PeriodsPanel canClose />);
    const trigger = within(await card("PRIMARIA · 1.er trimestre")).getByRole("button", { name: "Revisar y cerrar" });
    await user.click(trigger);

    await user.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Cancelar" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(trigger).toHaveFocus();
    expect(client.close).not.toHaveBeenCalled();
  });
});

describe("PeriodsPanel — Reabrir", () => {
  it("asks first, then reopens and reloads", async () => {
    const user = userEvent.setup();
    render(<PeriodsPanel canClose />);
    await user.click(within(await card("PRIMARIA · 2.º trimestre")).getByRole("button", { name: "Reabrir" }));

    const dialog = await screen.findByRole("dialog", { name: "Reabrir PRIMARIA · 2.º trimestre" });
    expect(within(dialog).getByText(/siguen viendo la boleta actual/)).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Reabrir periodo" }));

    expect(client.reopen).toHaveBeenCalledWith("t2");
    await waitFor(() => expect(client.list).toHaveBeenCalledTimes(2));
    expect(await screen.findByRole("status")).toHaveTextContent("Periodo reabierto");
  });
});

describe("PeriodsPanel — Ver boletas", () => {
  const BOLETA = { id: "rc1", studentId: "s1", studentName: "José Núñez", groupLabel: "1° PRIMARIA · A", version: 2, issuedAt: "2026-12-01T18:00:00Z", generalAverage: 8.7 };

  it("lists a group's boletas and downloads the PDF with its name", async () => {
    const user = userEvent.setup();
    client.reportCards.mockResolvedValue([BOLETA, { ...BOLETA, id: "rc2", studentName: "Ana Ruiz", version: 1, generalAverage: null }]);
    const createObjectURL = vi.fn(() => "blob:boleta");
    Object.assign(URL, { createObjectURL, revokeObjectURL: vi.fn() });
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
    render(<PeriodsPanel canClose />);
    await user.click(within(await card("PRIMARIA · 2.º trimestre")).getByRole("button", { name: "Ver boletas" }));

    const dialog = await screen.findByRole("dialog", { name: "Boletas · PRIMARIA · 2.º trimestre" });
    const groupSelect = await within(dialog).findByRole("combobox", { name: "Grupo" });
    expect(within(groupSelect).queryByRole("option", { name: "1° SECUNDARIA · A" })).toBeNull();
    await user.selectOptions(groupSelect, "g1");

    expect(client.reportCards).toHaveBeenCalledWith("t2", "g1");
    const row = await within(dialog).findByRole("row", { name: /José Núñez/ });
    expect(within(row).getByText("v2")).toBeInTheDocument();
    expect(within(row).getByText("8.7")).toBeInTheDocument();
    expect(within(within(dialog).getByRole("row", { name: /Ana Ruiz/ })).getByText("—")).toBeInTheDocument();

    await user.click(within(row).getByRole("button", { name: "Descargar PDF de José Núñez" }));

    await waitFor(() => expect(click).toHaveBeenCalled());
    expect(client.reportCardPdf).toHaveBeenCalledWith("rc1");
    const anchor = click.mock.contexts[0] as HTMLAnchorElement;
    expect(anchor.download).toBe("boleta-jose-nunez-2-o-trimestre.pdf");
    expect(anchor.href).toBe("blob:boleta");
  });

  it("says when a group has no boletas yet", async () => {
    const user = userEvent.setup();
    render(<PeriodsPanel canClose />);
    await user.click(within(await card("PRIMARIA · 2.º trimestre")).getByRole("button", { name: "Ver boletas" }));

    const dialog = await screen.findByRole("dialog");
    await user.selectOptions(await within(dialog).findByRole("combobox", { name: "Grupo" }), "g1");

    expect(await within(dialog).findByText("Este grupo aún no tiene boletas de este periodo.")).toBeInTheDocument();
  });
});
