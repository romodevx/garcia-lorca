import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../config/api";
import * as importApi from "../../studentImport/api";
import { StudentImportError, type StudentImportResult } from "../../studentImport/api";
import { LEVELS, MATCHED, importRow } from "./fixtures";
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

const config = vi.mocked(configApi);
const api = vi.mocked(importApi);
const ACTIVE = { id: "c1", name: "2026–2027", status: "active" as const, startsOn: "2026-08-01", endsOn: "2027-07-31" };

const FLAGGED: StudentImportResult = {
  ...MATCHED,
  rows: [
    importRow({ row: 2, status: "new" }),
    importRow({ row: 3, status: "warning", name: "RUIZ, BETO", warnings: ["phone_invalid"], issues: [{ field: "phone_mobile", code: "phone_invalid", original: "55 12", stored: "" }] }),
  ],
  summary: { total: 2, new: 2, warnings: 1, already: 0, skipped: 0, pending: 0, movedToCycleStart: 0 },
  planHash: "h-flagged",
};

beforeEach(() => {
  config.listCycles.mockResolvedValue([ACTIVE]);
  config.listLevels.mockResolvedValue(LEVELS);
  api.previewStudentImport.mockResolvedValue(FLAGGED);
});
afterEach(() => {
  vi.clearAllMocks();
  api.previewStudentImport.mockReset();
  api.commitStudentImport.mockReset();
});

function renderScreen(importReview: boolean) {
  render(<StudentImportScreen onBack={vi.fn()} onOpenTuition={vi.fn()} onViewStudents={vi.fn()} today="2026-10-21" importReview={importReview} />);
  return userEvent.setup();
}

async function upload(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.upload(await screen.findByLabelText("Archivo de Excel (.xlsx)"), new File([new Uint8Array([80, 75, 3, 4])], "Primaria.xlsx"));
  await screen.findByText("Vista previa: todavía no se ha guardado nada.");
}

const lastPreviewBody = () => api.previewStudentImport.mock.calls.at(-1)?.[1];

async function correctPhone(user: ReturnType<typeof userEvent.setup>, phone: string): Promise<void> {
  await user.click(screen.getByRole("tab", { name: /^Todos/ }));
  await user.click(screen.getByRole("button", { name: "Corregir fila 3" }));
  const field = screen.getByLabelText("Celular");
  await user.clear(field);
  await user.type(field, `${phone}{Enter}`);
}

describe("flag OFF", () => {
  it("never sends row decisions and offers none", async () => {
    const user = renderScreen(false);
    await upload(user);
    expect(lastPreviewBody()).toEqual({ fileName: "Primaria.xlsx", fileBase64: "UEsDBA==", levels: {}, capacities: {} });
    await user.click(screen.getByRole("tab", { name: /^Todos/ }));
    expect(screen.queryByRole("button", { name: "Corregir fila 3" })).not.toBeInTheDocument();
  });
});

describe("flag ON", () => {
  it("re-previews with each decision and commits the same ones with that preview's hash", async () => {
    const user = renderScreen(true);
    await upload(user);
    api.previewStudentImport.mockResolvedValue({ ...FLAGGED, planHash: "h-fixed" });

    await correctPhone(user, "5512345678");

    await waitFor(() => expect(lastPreviewBody()).toMatchObject({ rows: [{ row: 3, overrides: { mobilePhone: "5512345678" } }] }));
    api.commitStudentImport.mockResolvedValue({ ...FLAGGED, batchId: "b1" });
    await user.click(await screen.findByRole("button", { name: /^Importar 2 alumnos/ }));
    await waitFor(() => expect(api.commitStudentImport).toHaveBeenCalled());
    expect(api.commitStudentImport.mock.calls[0]?.[1]).toMatchObject({ planHash: "h-fixed", rows: [{ row: 3, overrides: { mobilePhone: "5512345678" } }] });
  });

  it("shows a refused correction in its row (422 IMPORT_ROWS_INVALID) and keeps the decision", async () => {
    const user = renderScreen(true);
    await upload(user);
    api.previewStudentImport.mockRejectedValueOnce(new StudentImportError(422, "{}", "IMPORT_ROWS_INVALID", [{ row: 3, field: "mobilePhone", code: "phone_invalid" }]));

    await correctPhone(user, "5512345678");

    expect(await screen.findByText("Celular: el teléfono lleva 10 dígitos")).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("Algunas correcciones no se aceptaron. Revísalas en la tabla.");
  });

  it("re-previews with the same decisions and says so when the commit finds the preview stale", async () => {
    const user = renderScreen(true);
    await upload(user);
    await user.click(screen.getByRole("tab", { name: /^Todos/ }));
    await user.click(screen.getByRole("button", { name: "Omitir fila 3" }));
    await waitFor(() => expect(lastPreviewBody()).toMatchObject({ rows: [{ row: 3, action: "omit" }] }));
    api.commitStudentImport.mockRejectedValue(new StudentImportError(409, "", "PREVIEW_STALE"));
    const calls = api.previewStudentImport.mock.calls.length;

    await user.click(screen.getByRole("button", { name: /^Importar/ }));

    expect(await screen.findByText("Algo cambió desde la vista previa; revísala de nuevo.")).toBeInTheDocument();
    await waitFor(() => expect(api.previewStudentImport.mock.calls.length).toBe(calls + 1));
    expect(lastPreviewBody()).toMatchObject({ rows: [{ row: 3, action: "omit" }] });
  });

  it("forgets the decisions when another file is chosen", async () => {
    const user = renderScreen(true);
    await upload(user);
    await user.click(screen.getByRole("tab", { name: /^Todos/ }));
    await user.click(screen.getByRole("button", { name: "Omitir fila 3" }));
    await user.click(screen.getByRole("button", { name: "Atrás" }));
    await upload(user);
    expect(lastPreviewBody()).toMatchObject({ rows: [] });
    expect(within(screen.getByRole("table")).queryByText("Omitida")).not.toBeInTheDocument();
  });
});
