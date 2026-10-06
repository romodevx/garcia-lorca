import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../config/api";
import * as csv from "../../_lib/csv";
import * as importApi from "../../studentImport/api";
import { StudentImportError } from "../../studentImport/api";
import { ALL_ALREADY, LEVELS, MATCHED, UNRESOLVED, importRow } from "./fixtures";
import { StudentImportScreen } from "./StudentImportScreen";

vi.mock("../../config/api", async (importOriginal) => ({
  ConfigError: (await importOriginal<typeof import("../../config/api")>()).ConfigError,
  listCycles: vi.fn(),
  listLevels: vi.fn(),
  createCycle: vi.fn(),
  activateCycle: vi.fn(),
}));
vi.mock("../../studentImport/api", async (importOriginal) => ({
  StudentImportError: (await importOriginal<typeof import("../../studentImport/api")>()).StudentImportError,
  previewStudentImport: vi.fn(),
  commitStudentImport: vi.fn(),
}));
vi.mock("../../_lib/csv", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../_lib/csv")>()),
  downloadCsv: vi.fn(),
}));

const config = vi.mocked(configApi);
const api = vi.mocked(importApi);
const ACTIVE = { id: "c1", name: "2026–2027", status: "active" as const, startsOn: "2026-08-01", endsOn: "2027-07-31" };
const DRAFT = { ...ACTIVE, id: "c2", status: "draft" as const };

beforeEach(() => {
  config.listCycles.mockResolvedValue([ACTIVE]);
  config.listLevels.mockResolvedValue(LEVELS);
  api.previewStudentImport.mockResolvedValue(MATCHED);
});
afterEach(() => vi.clearAllMocks());

function renderScreen() {
  const props = { onBack: vi.fn(), onOpenTuition: vi.fn(), onViewStudents: vi.fn(), today: "2026-10-06" };
  render(<StudentImportScreen {...props} />);
  return { ...props, user: userEvent.setup() };
}

const xlsx = (name = "Primaria.xlsx"): File => new File([new Uint8Array([80, 75, 3, 4])], name);

async function upload(user: ReturnType<typeof userEvent.setup>, file = xlsx()): Promise<void> {
  await user.upload(await screen.findByLabelText("Archivo de Excel (.xlsx)"), file);
}

describe("step 0 — the cycle", () => {
  it("asks for a cycle first when none is active, with a suggested name and Aug–Jul dates", async () => {
    config.listCycles.mockResolvedValue([]);
    config.createCycle.mockResolvedValue({ ...DRAFT, id: "c9" });
    config.activateCycle.mockResolvedValue({ ...ACTIVE, id: "c9" });
    const { user } = renderScreen();

    expect(await screen.findByText("Para migrar alumnos primero crea el ciclo escolar al que van a entrar")).toBeInTheDocument();
    expect(screen.getByLabelText("Nombre del ciclo")).toHaveValue("2026–2027");
    expect(screen.getByLabelText("Inicia")).toHaveValue("2026-08-01");
    expect(screen.getByLabelText("Termina")).toHaveValue("2027-07-31");

    await user.click(screen.getByRole("button", { name: "Crear y activar ciclo" }));

    expect(config.createCycle).toHaveBeenCalledWith(expect.any(String), { name: "2026–2027", startsOn: "2026-08-01", endsOn: "2027-07-31" });
    await waitFor(() => expect(config.activateCycle).toHaveBeenCalledWith(expect.any(String), "c9"));
    expect(await screen.findByText("Destino: Ciclo 2026–2027")).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 2, name: "Sube el archivo" })).toBeInTheDocument();
  });

  it("offers to activate an existing cycle instead of creating another", async () => {
    config.listCycles.mockResolvedValue([DRAFT]);
    config.activateCycle.mockResolvedValue({ ...DRAFT, status: "active" });
    const { user } = renderScreen();

    await user.click(await screen.findByRole("button", { name: "Activar «2026–2027»" }));

    expect(config.activateCycle).toHaveBeenCalledWith(expect.any(String), "c2");
    expect(config.createCycle).not.toHaveBeenCalled();
    expect(await screen.findByText("Destino: Ciclo 2026–2027")).toBeInTheDocument();
  });

  it("skips the step and names the destination when a cycle is active", async () => {
    renderScreen();
    expect(await screen.findByText("Destino: Ciclo 2026–2027")).toBeInTheDocument();
    expect(screen.queryByText(/primero crea el ciclo/)).not.toBeInTheDocument();
    expect(screen.getAllByRole("listitem").map((li) => li.textContent)).toEqual(["1Subir", "2Vista previa", "3Resultado"]);
  });
});

describe("step 1 — the file", () => {
  it("reads the .xlsx, previews it and moves focus to the preview's heading", async () => {
    const { user } = renderScreen();
    await upload(user);

    expect(await screen.findByText("Vista previa: todavía no se ha guardado nada.")).toBeInTheDocument();
    expect(api.previewStudentImport).toHaveBeenCalledWith(expect.any(String), { fileName: "Primaria.xlsx", fileBase64: "UEsDBA==", levels: {}, capacities: {} });
    expect(screen.getByRole("heading", { level: 2, name: "Vista previa" })).toHaveFocus();
  });

  it("takes only the first of several dropped files and says one at a time", async () => {
    renderScreen();
    const zone = await screen.findByRole("group", { name: "Zona para soltar el archivo" });
    fireEvent.drop(zone, { dataTransfer: { files: [xlsx("Maternal.xlsx"), xlsx("Primaria.xlsx")] } });

    expect(await screen.findByText("Sube un archivo a la vez")).toBeInTheDocument();
    await waitFor(() => expect(api.previewStudentImport).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({ fileName: "Maternal.xlsx" })));
  });

  it("refuses a non-.xlsx without sending it", async () => {
    renderScreen();
    const zone = await screen.findByRole("group", { name: "Zona para soltar el archivo" });
    fireEvent.drop(zone, { dataTransfer: { files: [xlsx("Primaria.xls")] } });

    expect(await screen.findByRole("alert")).toHaveTextContent("Este archivo no es un Excel (.xlsx).");
    expect(api.previewStudentImport).not.toHaveBeenCalled();
  });

  it("names the missing column the gateway reported", async () => {
    api.previewStudentImport.mockRejectedValue(new StudentImportError(400, "CURP", "FILE_MISSING_COLUMN"));
    const { user } = renderScreen();
    await upload(user);

    expect(await screen.findByRole("alert")).toHaveTextContent("Al archivo le falta la columna «CURP».");
    expect(screen.getByRole("heading", { level: 2, name: "Sube el archivo" })).toBeInTheDocument();
  });

  it("goes back to the cycle step when the gateway finds no active cycle", async () => {
    api.previewStudentImport.mockRejectedValue(new StudentImportError(409, "", "NO_ACTIVE_CYCLE"));
    // Active when the screen opened; gone by the time the gateway looked.
    config.listCycles.mockResolvedValueOnce([ACTIVE]).mockResolvedValue([]);
    const { user } = renderScreen();
    await upload(user);

    expect(await screen.findByText("Para migrar alumnos primero crea el ciclo escolar al que van a entrar")).toBeInTheDocument();
  });
});

describe("step 2 — choices re-preview", () => {
  it("sends the level choice and keeps sending it", async () => {
    api.previewStudentImport.mockResolvedValueOnce(UNRESOLVED);
    const { user } = renderScreen();
    await upload(user);

    const card = await screen.findByRole("region", { name: "Elige a dónde van los alumnos de «Preparatoria»" });
    await user.click(within(card).getByRole("button", { name: "Bachillerato" }));

    await waitFor(() => expect(api.previewStudentImport).toHaveBeenLastCalledWith(expect.any(String), expect.objectContaining({ levels: { Preparatoria: { levelId: "l-bach" } } })));
  });
});

describe("step 2 — answers arriving out of order", () => {
  // Two quick choices start two previews; if the older answer lands last it
  // must not repaint the screen with a plan the office already changed.
  it("keeps the answer of the latest choice", async () => {
    api.previewStudentImport.mockResolvedValueOnce(UNRESOLVED);
    let older: (r: importApi.StudentImportResult) => void = () => undefined;
    let newer: (r: importApi.StudentImportResult) => void = () => undefined;
    const { user } = renderScreen();
    await upload(user);
    const card = await screen.findByRole("region", { name: "Elige a dónde van los alumnos de «Preparatoria»" });

    api.previewStudentImport.mockReturnValueOnce(new Promise((resolve) => { older = resolve; }));
    await user.click(within(card).getByRole("button", { name: "Bachillerato" }));
    api.previewStudentImport.mockReturnValueOnce(new Promise((resolve) => { newer = resolve; }));
    await user.click(within(card).getByRole("button", { name: "+ Crear nivel nuevo «Preparatoria»" }));

    newer(MATCHED);
    await waitFor(() => expect(screen.queryByRole("region", { name: "Elige a dónde van los alumnos de «Preparatoria»" })).not.toBeInTheDocument());
    older(UNRESOLVED);
    await new Promise((r) => setTimeout(r, 0));

    expect(screen.queryByRole("region", { name: "Elige a dónde van los alumnos de «Preparatoria»" })).not.toBeInTheDocument();
  });
});

describe("step 3 — the commit", () => {
  it("commits the same file with the planHash, then shows the result and the report", async () => {
    let finish: (r: importApi.StudentImportResult) => void = () => undefined;
    api.commitStudentImport.mockReturnValue(new Promise((resolve) => { finish = resolve; }));
    const { user } = renderScreen();
    await upload(user);
    const addSpy = vi.spyOn(window, "addEventListener");

    await user.click(await screen.findByRole("button", { name: "Importar 2 alumnos a Primaria" }));

    expect(api.commitStudentImport).toHaveBeenCalledWith(expect.any(String), { fileName: "Primaria.xlsx", fileBase64: "UEsDBA==", levels: {}, capacities: {}, planHash: "h1" });
    expect(screen.getByText("Importando 2 alumnos… no cierres esta ventana")).toBeInTheDocument();
    expect(addSpy).toHaveBeenCalledWith("beforeunload", expect.any(Function));

    finish({ ...MATCHED, batchId: "b1", rows: MATCHED.rows.map((r) => (r.status === "new" ? { ...r, enrollmentNumber: "2026-00042" } : r)) });
    expect(await screen.findByText("Primaria quedó migrada.")).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 2, name: "Resultado" })).toHaveFocus();

    await user.click(screen.getByRole("button", { name: "Descargar reporte CSV" }));
    const [name, body] = vi.mocked(csv.downloadCsv).mock.calls[0] as [string, string];
    expect(name).toMatch(/^migracion-Primaria-/);
    expect(body.split("\r\n")).toHaveLength(MATCHED.rows.length + 1);
    expect(body).toContain("2026-00042");
  });

  it("starts over with «Subir otro archivo»", async () => {
    api.commitStudentImport.mockResolvedValue({ ...MATCHED, batchId: "b1" });
    const { user } = renderScreen();
    await upload(user);
    await user.click(await screen.findByRole("button", { name: "Importar 2 alumnos a Primaria" }));
    await user.click(await screen.findByRole("button", { name: "Subir otro archivo" }));

    expect(screen.getByRole("heading", { level: 2, name: "Sube el archivo" })).toHaveFocus();
  });

  it("shows the imported pupils filtered by the level", async () => {
    api.commitStudentImport.mockResolvedValue({ ...MATCHED, batchId: "b1" });
    const { user, onViewStudents } = renderScreen();
    await upload(user);
    await user.click(await screen.findByRole("button", { name: "Importar 2 alumnos a Primaria" }));
    await user.click(await screen.findByRole("button", { name: "Ver alumnos importados" }));

    expect(onViewStudents).toHaveBeenCalledWith("l-pri");
  });

  it("on a stale preview says so and previews again", async () => {
    api.commitStudentImport.mockRejectedValue(new StudentImportError(409, "", "PREVIEW_STALE"));
    const { user } = renderScreen();
    await upload(user);
    await user.click(await screen.findByRole("button", { name: "Importar 2 alumnos a Primaria" }));

    expect(await screen.findByText("Algo cambió desde la vista previa; revísala de nuevo.")).toBeInTheDocument();
    expect(api.previewStudentImport).toHaveBeenCalledTimes(2);
  });

  it("on a cut connection says re-uploading the same file is safe", async () => {
    api.commitStudentImport.mockRejectedValue(new TypeError("Failed to fetch"));
    const { user } = renderScreen();
    await upload(user);
    await user.click(await screen.findByRole("button", { name: "Importar 2 alumnos a Primaria" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Sube el mismo archivo otra vez");
  });

  it("counts what went in when the answer came back cut", async () => {
    api.commitStudentImport.mockResolvedValue({
      ...MATCHED,
      rows: [importRow({ row: 2, status: "new" }), importRow({ row: 3, status: "skipped", reason: "not_answered" })],
      summary: { total: 2, new: 1, warnings: 0, already: 0, skipped: 1, pending: 0 },
    });
    const { user } = renderScreen();
    await upload(user);
    await user.click(await screen.findByRole("button", { name: "Importar 2 alumnos a Primaria" }));

    expect(await screen.findByText(/^Se importaron 1 de 2 y la conexión se cortó/)).toBeInTheDocument();
  });

  it("offers nothing to import when all were already there", async () => {
    api.previewStudentImport.mockResolvedValue(ALL_ALREADY);
    const { user } = renderScreen();
    await upload(user);

    expect(await screen.findByText("Estos 2 alumnos ya estaban registrados (misma CURP). No se duplicó nada.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Importar/ })).not.toBeInTheDocument();
  });
});
