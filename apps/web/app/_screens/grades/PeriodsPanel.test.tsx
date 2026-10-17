import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../config/api";
import { GradeRequestError } from "../../grades/requestError";
import * as periodsApi from "../../grades/periods";
import type { GradingPeriod, PeriodCaptureProgress, PeriodsClient } from "../../grades/periods";
import { downloadBlob } from "../../_lib/downloadBlob";
import { PeriodsPanel } from "./PeriodsPanel";

vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  listGradeLevels: vi.fn(),
  listLabelledGroups: vi.fn(),
}));
vi.mock(import("../../grades/periods"), async (original) => ({ ...(await original()), periodsClient: vi.fn() }));
vi.mock("../../_lib/downloadBlob", () => ({ downloadBlob: vi.fn() }));

const config = vi.mocked(configApi);
const periods = vi.mocked(periodsApi);

const OPEN: GradingPeriod = {
  termId: "t1", termName: "1.er trimestre", ordinal: 1, levelId: "l1", levelName: "PRIMARIA",
  startsOn: "2026-08-24", endsOn: "2026-11-30", status: "open", version: 0, progress: { captured: 412, expected: 450 },
};
const CLOSED: GradingPeriod = {
  termId: "t2", termName: "2.º trimestre", ordinal: 2, levelId: "l1", levelName: "PRIMARIA",
  startsOn: "2026-12-07", endsOn: "2027-03-26", status: "closed", closedAt: "2026-12-01T18:00:00Z", closedByName: "Luis Beltrán",
  version: 2, progress: { captured: 450, expected: 450 },
};
const SECUNDARIA: GradingPeriod = { ...OPEN, termId: "t4", levelId: "l2", levelName: "SECUNDARIA", progress: { captured: 0, expected: 90 } };

const MISSING: PeriodCaptureProgress = {
  termId: "t1", unfinished: false, endsOn: "2026-11-30", groupsWithoutPeriod: [], expected: 450, captured: 412,
  reportCards: 24, skippedWithoutGrades: 1, withdrawnReportCards: 0,
  missing: [
    { groupId: "g1", groupLabel: "1° PRIMARIA · A", subjectId: "s1", subjectName: "Matemáticas", teacherName: "Ada Lovelace", missingCount: 30 },
    { groupId: "g1", groupLabel: "1° PRIMARIA · A", subjectId: "s2", subjectName: "Historia", teacherName: "Alan Turing", missingCount: 5 },
    { groupId: "g2", groupLabel: "2° PRIMARIA · A", subjectId: "s3", subjectName: "Español", teacherName: "Grace Hopper", missingCount: 3 },
  ],
};
const COMPLETE: PeriodCaptureProgress = { ...MISSING, captured: 450, missing: [] };

type MockClient = { [K in keyof PeriodsClient]: ReturnType<typeof vi.fn> };
let client: MockClient;

beforeEach(() => {
  client = {
    list: vi.fn().mockResolvedValue([OPEN, CLOSED, SECUNDARIA]),
    progress: vi.fn().mockResolvedValue(MISSING),
    close: vi.fn().mockResolvedValue({ status: "closed", version: 1, reportCards: 124, skippedWithoutGrades: 0, withdrawnReportCards: 0 }),
    reopen: vi.fn().mockResolvedValue({ status: "open" }),
    reportCards: vi.fn().mockResolvedValue([]),
    reportCardPdf: vi.fn().mockResolvedValue({ blob: new Blob(["%PDF"], { type: "application/pdf" }) }),
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
  vi.restoreAllMocks();
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});

const card = async (name: string): Promise<HTMLElement> => screen.findByRole("article", { name });

/** A reopen needs its reason (CC-308): type one into the open dialog. */
async function giveReason(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.type(within(await screen.findByRole("dialog")).getByRole("textbox", { name: /Motivo/ }), "Corrección");
}

describe("PeriodsPanel — cards", () => {
  it("shows the active cycle's periods per level, with status, dates and capture progress", async () => {
    render(<PeriodsPanel canClose />);

    const open = await card("PRIMARIA · 1.er trimestre");
    expect(client.list).toHaveBeenCalledWith("c1");
    expect(within(open).getByText("Abierto")).toBeInTheDocument();
    expect(within(open).getByText("24 ago 2026 – 30 nov 2026")).toBeInTheDocument();
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
    expect(within(closed).getByRole("button", { name: /^Ver boletas/ })).toBeInTheDocument();
    expect(within(closed).queryByRole("button", { name: /^Revisar y cerrar/ })).toBeNull();
  });

  it("names each card's actions with its level and period, so nine buttons never sound alike (CC-312)", async () => {
    render(<PeriodsPanel canClose />);

    const open = await card("PRIMARIA · 1.er trimestre");
    expect(within(open).getByRole("button", { name: "Revisar y cerrar PRIMARIA · 1.er trimestre" })).toHaveTextContent("Revisar y cerrar");
    const closed = await card("PRIMARIA · 2.º trimestre");
    expect(within(closed).getByRole("button", { name: "Ver boletas PRIMARIA · 2.º trimestre" })).toHaveTextContent("Ver boletas");
  });

  it("without grades:close there is nothing to close or reopen", async () => {
    render(<PeriodsPanel canClose={false} />);

    const open = await card("PRIMARIA · 1.er trimestre");
    expect(within(open).queryByRole("button", { name: /^Revisar y cerrar/ })).toBeNull();
    expect(within(await card("PRIMARIA · 2.º trimestre")).queryByRole("button", { name: "Reabrir" })).toBeNull();
    expect(within(await card("PRIMARIA · 2.º trimestre")).getByRole("button", { name: /^Ver boletas/ })).toBeInTheDocument();
  });

  it("shows only the levels the server lists: an ungraded Kínder it omits gets no section (T10)", async () => {
    config.listGradeLevels.mockResolvedValue([
      { id: "k1", levelId: "lk", name: "1° KINDER", ordinal: 1 },
      { id: "p1", levelId: "l1", name: "1° PRIMARIA", ordinal: 1 },
    ]);
    client.list.mockResolvedValue([OPEN]);
    render(<PeriodsPanel canClose />);

    expect(await card("PRIMARIA · 1.er trimestre")).toBeInTheDocument();
    expect(screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent)).toEqual(["PRIMARIA"]);
    expect(screen.queryByText(/KINDER/i)).toBeNull();
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
    await user.click(within(await card("PRIMARIA · 1.er trimestre")).getByRole("button", { name: /^Revisar y cerrar/ }));

    const dialog = await screen.findByRole("dialog", { name: "Cerrar PRIMARIA · 1.er trimestre" });
    expect(client.progress).toHaveBeenCalledWith("t1");
    expect(await within(dialog).findByText("412 de 450 capturadas")).toBeInTheDocument();
    const groupA = within(dialog).getByRole("group", { name: "1° PRIMARIA · A" });
    expect(within(groupA).getByText("Matemáticas · Ada Lovelace · faltan 30")).toBeInTheDocument();
    expect(within(groupA).getByText("Historia · Alan Turing · faltan 5")).toBeInTheDocument();
    expect(within(dialog).getByRole("group", { name: "2° PRIMARIA · A" })).toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: "Cerrar periodo" })).toBeNull();
    expect(within(dialog).getByText("Se publicarán 24 boletas. 1 alumno sin calificaciones no recibirá boleta.")).toBeInTheDocument();
    expect(within(dialog).queryByText(/una boleta por alumno/)).toBeNull();

    client.close.mockResolvedValueOnce({ status: "closed", version: 1, reportCards: 24, skippedWithoutGrades: 1, withdrawnReportCards: 0 });
    await user.click(within(dialog).getByRole("button", { name: "Cerrar de todos modos" }));

    expect(client.close).toHaveBeenCalledWith("t1", true);
    expect(await screen.findByRole("status")).toHaveTextContent("Se publicaron 24 boletas. 1 alumno sin calificaciones no recibió boleta.");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(client.list).toHaveBeenCalledTimes(2);
  });

  it("with nothing missing it closes normally", async () => {
    const user = userEvent.setup();
    client.progress.mockResolvedValue(COMPLETE);
    render(<PeriodsPanel canClose />);
    await user.click(within(await card("PRIMARIA · 1.er trimestre")).getByRole("button", { name: /^Revisar y cerrar/ }));

    const dialog = await screen.findByRole("dialog");
    await user.click(await within(dialog).findByRole("button", { name: "Cerrar periodo" }));

    expect(within(dialog).queryByRole("button", { name: "Cerrar de todos modos" })).toBeNull();
    expect(client.close).toHaveBeenCalledWith("t1", false);
    expect(await screen.findByRole("status")).toHaveTextContent("Se publicaron 124 boletas");
  });

  it("a re-close warns before and after that it withdraws a boleta a family already saw", async () => {
    const user = userEvent.setup();
    client.progress.mockResolvedValue({ ...COMPLETE, reportCards: 2, skippedWithoutGrades: 0, withdrawnReportCards: 1 });
    client.close.mockResolvedValueOnce({ status: "closed", version: 2, reportCards: 2, skippedWithoutGrades: 0, withdrawnReportCards: 1 });
    render(<PeriodsPanel canClose />);
    await user.click(within(await card("PRIMARIA · 1.er trimestre")).getByRole("button", { name: /^Revisar y cerrar/ }));

    const dialog = await screen.findByRole("dialog");
    expect(
      await within(dialog).findByText("Se publicarán 2 boletas. Se retirará 1 boleta ya publicada a un alumno que ya no tiene calificaciones."),
    ).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Cerrar periodo" }));

    expect(await screen.findByRole("status")).toHaveTextContent(
      "Se publicaron 2 boletas. Se retiró 1 boleta ya publicada a un alumno que ya no tiene calificaciones.",
    );
  });

  it("a MISSING_GRADES refusal (a capture was deleted meanwhile) shows the server's words and the missing list again", async () => {
    const user = userEvent.setup();
    client.progress.mockResolvedValueOnce(COMPLETE).mockResolvedValueOnce(MISSING);
    client.close.mockRejectedValueOnce(new GradeRequestError(409, "Faltan 38 calificaciones por capturar", "MISSING_GRADES"));
    render(<PeriodsPanel canClose />);
    await user.click(within(await card("PRIMARIA · 1.er trimestre")).getByRole("button", { name: /^Revisar y cerrar/ }));

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
    const trigger = within(await card("PRIMARIA · 1.er trimestre")).getByRole("button", { name: /^Revisar y cerrar/ });
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
    await user.type(within(dialog).getByRole("textbox", { name: /Motivo/ }), "  Corrección de Matemáticas de 3º B  ");
    await user.click(within(dialog).getByRole("button", { name: "Reabrir periodo" }));

    expect(client.reopen).toHaveBeenCalledWith("t2", "Corrección de Matemáticas de 3º B");
    await waitFor(() => expect(client.list).toHaveBeenCalledTimes(2));
    expect(await screen.findByRole("status")).toHaveTextContent("Periodo reabierto");
  });
});

describe("PeriodsPanel — Reabrir pide el motivo (CC-308)", () => {
  it("without a reason it says what is missing and reopens nothing", async () => {
    const user = userEvent.setup();
    render(<PeriodsPanel canClose />);
    await user.click(within(await card("PRIMARIA · 2.º trimestre")).getByRole("button", { name: "Reabrir" }));
    const dialog = await screen.findByRole("dialog");
    await user.type(within(dialog).getByRole("textbox", { name: /Motivo/ }), "   ");
    await user.click(within(dialog).getByRole("button", { name: "Reabrir periodo" }));

    expect(await within(dialog).findByText("Falta este dato")).toBeInTheDocument();
    expect(within(dialog).getByRole("textbox", { name: /Motivo/ })).toHaveAttribute("aria-invalid", "true");
    expect(client.reopen).not.toHaveBeenCalled();
  });

  it("over 500 characters it says the limit and reopens nothing", async () => {
    const user = userEvent.setup();
    render(<PeriodsPanel canClose />);
    await user.click(within(await card("PRIMARIA · 2.º trimestre")).getByRole("button", { name: "Reabrir" }));
    const dialog = await screen.findByRole("dialog");
    const field = within(dialog).getByRole("textbox", { name: /Motivo/ });
    await user.click(field);
    await user.paste("a".repeat(501));
    await user.click(within(dialog).getByRole("button", { name: "Reabrir periodo" }));

    expect(await within(dialog).findByText("Máximo 500 caracteres")).toBeInTheDocument();
    expect(client.reopen).not.toHaveBeenCalled();
  });
});

describe("PeriodsPanel — Ver boletas", () => {
  const BOLETA = { id: "rc1", studentId: "s1", studentName: "José Núñez", groupLabel: "1° PRIMARIA · A", version: 2, issuedAt: "2026-12-01T18:00:00Z", generalAverage: 8.7, hasGrades: true };

  const openGroup = async (user: ReturnType<typeof userEvent.setup>): Promise<HTMLElement> => {
    await user.click(within(await card("PRIMARIA · 2.º trimestre")).getByRole("button", { name: /^Ver boletas/ }));
    const dialog = await screen.findByRole("dialog", { name: "Boletas · PRIMARIA · 2.º trimestre" });
    const groupSelect = await within(dialog).findByRole("combobox", { name: "Grupo" });
    expect(within(groupSelect).queryByRole("option", { name: "1° SECUNDARIA · A" })).toBeNull();
    await user.selectOptions(groupSelect, "g1");
    return dialog;
  };

  it("lists a group's boletas and downloads the PDF with the server's file name", async () => {
    const user = userEvent.setup();
    const blob = new Blob(["%PDF"], { type: "application/pdf" });
    client.reportCardPdf.mockResolvedValue({ blob, fileName: "boleta-2026-00042-2-o-trimestre-v2.pdf" });
    client.reportCards.mockResolvedValue([
      BOLETA,
      { ...BOLETA, id: "rc2", studentName: "Ana Ruiz", version: 1, generalAverage: null },
      { ...BOLETA, id: "rc3", studentName: "Luz Paz", generalAverage: 8.66 },
      { ...BOLETA, id: "rc4", studentName: "Iker Sol", generalAverage: 9 },
    ]);
    render(<PeriodsPanel canClose />);
    const dialog = await openGroup(user);

    expect(client.reportCards).toHaveBeenCalledWith("t2", "g1");
    const row = await within(dialog).findByRole("row", { name: /José Núñez/ });
    expect(within(row).getByText("v2")).toBeInTheDocument();
    expect(within(row).getByText("8.7")).toBeInTheDocument();
    expect(within(within(dialog).getByRole("row", { name: /Luz Paz/ })).getByText("8.66")).toBeInTheDocument();
    expect(within(within(dialog).getByRole("row", { name: /Iker Sol/ })).getByText("9")).toBeInTheDocument();
    expect(within(within(dialog).getByRole("row", { name: /Ana Ruiz/ })).getByText("—")).toBeInTheDocument();

    await user.click(within(row).getByRole("button", { name: "Descargar PDF de José Núñez" }));

    await waitFor(() => expect(downloadBlob).toHaveBeenCalledWith(blob, "boleta-2026-00042-2-o-trimestre-v2.pdf"));
    expect(client.reportCardPdf).toHaveBeenCalledWith("rc1");
  });

  it("a boleta without any grade says so in words instead of a dash (it is still issued)", async () => {
    const user = userEvent.setup();
    client.reportCards.mockResolvedValue([{ ...BOLETA, id: "rc2", studentName: "Ana Ruiz", generalAverage: null, hasGrades: false }]);
    render(<PeriodsPanel canClose />);
    const dialog = await openGroup(user);

    const row = await within(dialog).findByRole("row", { name: /Ana Ruiz/ });
    expect(within(row).getByText("Sin calificaciones capturadas en este periodo")).toBeInTheDocument();
    expect(within(row).queryByText("—")).toBeNull();
    expect(within(row).getByRole("button", { name: "Descargar PDF de Ana Ruiz" })).toBeInTheDocument();
  });

  it("without the server's file name it names the PDF boleta-<alumno>-<periodo>-v<N>.pdf", async () => {
    const user = userEvent.setup();
    client.reportCards.mockResolvedValue([BOLETA]);
    render(<PeriodsPanel canClose />);
    const dialog = await openGroup(user);

    await user.click(await within(dialog).findByRole("button", { name: "Descargar PDF de José Núñez" }));

    await waitFor(() => expect(downloadBlob).toHaveBeenCalledWith(expect.any(Blob), "boleta-jose-nunez-2-o-trimestre-v2.pdf"));
  });

  it("says when a group has no boletas yet", async () => {
    const user = userEvent.setup();
    render(<PeriodsPanel canClose />);
    await user.click(within(await card("PRIMARIA · 2.º trimestre")).getByRole("button", { name: /^Ver boletas/ }));

    const dialog = await screen.findByRole("dialog");
    await user.selectOptions(await within(dialog).findByRole("combobox", { name: "Grupo" }), "g1");

    expect(await within(dialog).findByText("Este grupo aún no tiene boletas de este periodo.")).toBeInTheDocument();
  });
});

const openClose = async (user: ReturnType<typeof userEvent.setup>, name = "PRIMARIA · 1.er trimestre"): Promise<HTMLElement> => {
  await user.click(within(await card(name)).getByRole("button", { name: /^Revisar y cerrar/ }));
  return screen.findByRole("dialog");
};

describe("PeriodsPanel — a period that has not finished", () => {
  const SPRING: GradingPeriod = {
    ...OPEN, termId: "t3", termName: "3.er trimestre", ordinal: 3, startsOn: "2027-04-12", endsOn: "2027-07-09", progress: { captured: 0, expected: 450 },
  };

  it("warns it has not finished instead of calling the capture complete, and only offers closing anyway", async () => {
    const user = userEvent.setup();
    client.list.mockResolvedValue([SPRING]);
    client.progress.mockResolvedValue({ ...COMPLETE, termId: "t3", unfinished: true, endsOn: "2027-07-09" });
    render(<PeriodsPanel canClose />);
    const dialog = await openClose(user, "PRIMARIA · 3.er trimestre");

    expect(await within(dialog).findByText("Aún no termina el periodo (termina el 9 jul 2027)")).toBeInTheDocument();
    expect(within(dialog).queryByText("Todas las calificaciones están capturadas.")).toBeNull();
    expect(within(dialog).queryByRole("button", { name: "Cerrar periodo" })).toBeNull();
    await user.click(within(dialog).getByRole("button", { name: "Cerrar de todos modos" }));
    expect(client.close).toHaveBeenCalledWith("t3", true);
  });
});

describe("PeriodsPanel — groups whose grade lacks the period", () => {
  it("names them, explains where to configure it, and offers no close the server would refuse", async () => {
    const user = userEvent.setup();
    client.progress.mockResolvedValue({ ...COMPLETE, groupsWithoutPeriod: ["2° PRIMARIA · A", "3° PRIMARIA · B"] });
    render(<PeriodsPanel canClose />);
    const dialog = await openClose(user);

    expect(await within(dialog).findByText("Sin periodo 1 configurado: 2° PRIMARIA · A, 3° PRIMARIA · B")).toBeInTheDocument();
    expect(within(dialog).getByText(/no se puede cerrar hasta que Configuración › Periodos por grado/)).toBeInTheDocument();
    expect(within(dialog).queryByText("Todas las calificaciones están capturadas.")).toBeNull();
    expect(within(dialog).queryByRole("button", { name: "Cerrar periodo" })).toBeNull();
    expect(within(dialog).queryByRole("button", { name: "Cerrar de todos modos" })).toBeNull();
  });
});

describe("PeriodsPanel — closing honestly", () => {
  it("with nothing expected it never says the capture is complete", async () => {
    const user = userEvent.setup();
    client.progress.mockResolvedValue({ ...COMPLETE, expected: 0, captured: 0 });
    render(<PeriodsPanel canClose />);
    const dialog = await openClose(user);

    expect(await within(dialog).findByRole("button", { name: "Cerrar de todos modos" })).toBeInTheDocument();
    expect(within(dialog).queryByText("Todas las calificaciones están capturadas.")).toBeNull();
    expect(within(dialog).getByText(/no hay calificaciones que revisar/i)).toBeInTheDocument();
  });

  it("lists two subjects that share a name as two rows (keyed by subject id)", async () => {
    const user = userEvent.setup();
    const errors = vi.spyOn(console, "error").mockImplementation(() => {});
    client.progress.mockResolvedValue({
      ...MISSING,
      missing: [
        { groupId: "g1", groupLabel: "1° PRIMARIA · A", subjectId: "s1", subjectName: "Taller", teacherName: "Ada Lovelace", missingCount: 2 },
        { groupId: "g1", groupLabel: "1° PRIMARIA · A", subjectId: "s2", subjectName: "Taller", teacherName: "Ada Lovelace", missingCount: 3 },
      ],
    });
    render(<PeriodsPanel canClose />);
    const dialog = await openClose(user);

    expect(await within(dialog).findByText("Taller · Ada Lovelace · faltan 2")).toBeInTheDocument();
    expect(within(dialog).getByText("Taller · Ada Lovelace · faltan 3")).toBeInTheDocument();
    expect(errors.mock.calls.flat().join(" ")).not.toMatch(/same key/);
  });
});

describe("PeriodsPanel — refusals that mean something", () => {
  it("ALREADY_CLOSED: someone closed it meanwhile — reload, dismiss and say so", async () => {
    const user = userEvent.setup();
    client.progress.mockResolvedValue(COMPLETE);
    client.close.mockRejectedValueOnce(new GradeRequestError(409, "El periodo ya está cerrado", "ALREADY_CLOSED"));
    render(<PeriodsPanel canClose />);
    const dialog = await openClose(user);
    await user.click(await within(dialog).findByRole("button", { name: "Cerrar periodo" }));

    expect(await screen.findByRole("status")).toHaveTextContent("El periodo ya estaba cerrado");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(client.list).toHaveBeenCalledTimes(2);
  });

  it("ALREADY_OPEN on reopen: reload, dismiss and say so", async () => {
    const user = userEvent.setup();
    client.reopen.mockRejectedValueOnce(new GradeRequestError(409, "El periodo ya está abierto", "ALREADY_OPEN"));
    render(<PeriodsPanel canClose />);
    await user.click(within(await card("PRIMARIA · 2.º trimestre")).getByRole("button", { name: "Reabrir" }));
    await giveReason(user);
    await user.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Reabrir periodo" }));

    expect(await screen.findByRole("status")).toHaveTextContent("El periodo ya estaba abierto");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(client.list).toHaveBeenCalledTimes(2);
  });

  it("PERIOD_NOT_CONFIGURED says which group lacks the period and where to give it one", async () => {
    const user = userEvent.setup();
    client.progress.mockResolvedValue(COMPLETE);
    client.close.mockRejectedValueOnce(new GradeRequestError(409, "El grupo 2° PRIMARIA · A no tiene el periodo 1 configurado", "PERIOD_NOT_CONFIGURED"));
    render(<PeriodsPanel canClose />);
    const dialog = await openClose(user);
    await user.click(await within(dialog).findByRole("button", { name: "Cerrar periodo" }));

    const alert = await within(dialog).findByRole("alert");
    expect(alert).toHaveTextContent("El grupo 2° PRIMARIA · A no tiene el periodo 1 configurado");
    expect(alert).toHaveTextContent("Configuración › Periodos por grado");
  });

  it("LEVEL_NOT_GRADED says the level has no numeric grades", async () => {
    const user = userEvent.setup();
    client.progress.mockResolvedValue(COMPLETE);
    client.close.mockRejectedValueOnce(new GradeRequestError(409, "", "LEVEL_NOT_GRADED"));
    render(<PeriodsPanel canClose />);
    const dialog = await openClose(user);
    await user.click(await within(dialog).findByRole("button", { name: "Cerrar periodo" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent(/no se califica con números/);
  });
});

describe("PeriodsPanel — focus and double submits", () => {
  it("after closing, the focus lands on the period's heading, never on the page body", async () => {
    const user = userEvent.setup();
    client.progress.mockResolvedValue(COMPLETE);
    client.list.mockResolvedValueOnce([OPEN, CLOSED]).mockResolvedValue([{ ...OPEN, status: "closed", version: 1 }, CLOSED]);
    render(<PeriodsPanel canClose />);
    const dialog = await openClose(user);
    await user.click(await within(dialog).findByRole("button", { name: "Cerrar periodo" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() => expect(within(screen.getByRole("article", { name: "PRIMARIA · 1.er trimestre" })).getByText("Cerrado v1")).toBeInTheDocument());
    expect(within(screen.getByRole("article", { name: "PRIMARIA · 1.er trimestre" })).getByRole("heading", { name: "1.er trimestre" })).toHaveFocus();
  });

  it("after reopening, the focus lands on the period's heading", async () => {
    const user = userEvent.setup();
    client.list.mockResolvedValueOnce([OPEN, CLOSED]).mockResolvedValue([OPEN, { ...CLOSED, status: "open" }]);
    render(<PeriodsPanel canClose />);
    await user.click(within(await card("PRIMARIA · 2.º trimestre")).getByRole("button", { name: "Reabrir" }));
    await giveReason(user);
    await user.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Reabrir periodo" }));

    await waitFor(() => expect(within(screen.getByRole("article", { name: "PRIMARIA · 2.º trimestre" })).getByText("Abierto")).toBeInTheDocument());
    expect(within(screen.getByRole("article", { name: "PRIMARIA · 2.º trimestre" })).getByRole("heading", { name: "2.º trimestre" })).toHaveFocus();
  });

  it("two clicks on Cerrar periodo in the same instant close once", async () => {
    const user = userEvent.setup();
    client.progress.mockResolvedValue(COMPLETE);
    client.close.mockReturnValue(new Promise(() => {}));
    render(<PeriodsPanel canClose />);
    const dialog = await openClose(user);
    const button = await within(dialog).findByRole("button", { name: "Cerrar periodo" });

    act(() => {
      button.click();
      button.click();
    });

    expect(client.close).toHaveBeenCalledTimes(1);
  });

  it("two clicks on Reabrir periodo in the same instant reopen once", async () => {
    const user = userEvent.setup();
    client.reopen.mockReturnValue(new Promise(() => {}));
    render(<PeriodsPanel canClose />);
    await user.click(within(await card("PRIMARIA · 2.º trimestre")).getByRole("button", { name: "Reabrir" }));
    await giveReason(user);
    const button = within(await screen.findByRole("dialog")).getByRole("button", { name: "Reabrir periodo" });

    act(() => {
      button.click();
      button.click();
    });

    expect(client.reopen).toHaveBeenCalledTimes(1);
  });
});

describe("PeriodsPanel — a period no grade has configured", () => {
  it("offers no close; it says where to configure it", async () => {
    client.list.mockResolvedValue([{ ...OPEN, startsOn: null, endsOn: null, progress: { captured: 0, expected: 0 } }]);
    render(<PeriodsPanel canClose />);

    const open = await card("PRIMARIA · 1.er trimestre");
    expect(within(open).queryByRole("button", { name: /^Revisar y cerrar/ })).toBeNull();
    expect(within(open).getByText("Sin fechas: ningún grado tiene este periodo")).toBeInTheDocument();
    expect(within(open).getByText("Dale fechas en Configuración › Periodos por grado")).toBeInTheDocument();
  });
});
