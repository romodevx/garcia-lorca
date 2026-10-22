import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { RowFieldError, StudentImportResult } from "../../studentImport/api";
import type { Decisions } from "../../studentImport/review";
import { PreviewStep, type PreviewReview } from "./PreviewStep";
import { LEVELS, MATCHED, importRow } from "./fixtures";

/** A file with one flagged phone (row 3), one row without CURP (row 4) and a plain one (row 2). */
const REVIEWED: StudentImportResult = {
  ...MATCHED,
  rows: [
    importRow({ row: 2, status: "new" }),
    importRow({
      row: 3, status: "warning", name: "RUIZ, BETO", curp: "RUXB100101HDFZXT02", warnings: ["phone_invalid"],
      issues: [{ field: "phone_mobile", code: "phone_invalid", original: "55 12", stored: "" }],
    }),
    importRow({
      row: 4, status: "skipped", reason: "curp_invalid", name: "LUNA, DANI", curp: "",
      canDraft: true, issues: [{ field: "curp", code: "curp_missing", original: "", stored: "" }],
    }),
  ],
  summary: { total: 3, new: 2, warnings: 1, already: 0, skipped: 1, pending: 0, movedToCycleStart: 0 },
};

function renderPreview(result: StudentImportResult, review?: Partial<PreviewReview>) {
  const handlers: PreviewReview = { decisions: {}, rowErrors: [], onAction: vi.fn(), onCorrect: vi.fn(), today: "2026-10-21", ...review };
  render(
    <PreviewStep
      result={result}
      levels={LEVELS}
      onChoose={vi.fn()}
      onCapacity={vi.fn()}
      onImport={vi.fn()}
      onBack={vi.fn()}
      onCancel={vi.fn()}
      onOpenTuition={vi.fn()}
      focusUnresolved={0}
      review={review === undefined ? undefined : handlers}
    />,
  );
  return { ...handlers, user: userEvent.setup() };
}

const tableOf = () => screen.getByRole("table");
const rowOf = (n: number) => within(tableOf()).getAllByRole("row").find((r) => within(r).queryAllByRole("cell")[0]?.textContent === String(n));

async function openAll(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("tab", { name: /^Todos/ }));
}

describe("flag OFF: the preview is exactly as before", () => {
  it("shows no review actions and no review cards", async () => {
    const { user } = renderPreview(REVIEWED);
    await openAll(user);
    expect(screen.queryByRole("button", { name: /Corregir/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Omitir/ })).not.toBeInTheDocument();
    expect(screen.queryByText("Borradores")).not.toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: "Acciones" })).not.toBeInTheDocument();
  });
});

describe("flag ON: each flagged row says why and what can be done", () => {
  it("says each issue in plain Spanish, with the file's value", async () => {
    const { user } = renderPreview(REVIEWED, {});
    await openAll(user);
    expect(within(rowOf(3) as HTMLElement).getByText("Celular: Teléfono incompleto (en el Excel: «55 12»)")).toBeInTheDocument();
    expect(within(rowOf(4) as HTMLElement).getByText("CURP: Sin CURP")).toBeInTheDocument();
  });

  it("offers Corregir and Omitir on a flagged row, and the draft only when it can be one", async () => {
    const { user } = renderPreview(REVIEWED, {});
    await openAll(user);
    expect(screen.getByRole("button", { name: "Corregir fila 3" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Omitir fila 3" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Importar fila 3 como borrador" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Importar fila 4 como borrador" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Corregir fila 2" })).not.toBeInTheDocument();
  });

  it("omits and drafts through the screen", async () => {
    const { user, onAction } = renderPreview(REVIEWED, {});
    await openAll(user);
    await user.click(screen.getByRole("button", { name: "Omitir fila 3" }));
    expect(onAction).toHaveBeenCalledWith(3, "omit");
    await user.click(screen.getByRole("button", { name: "Importar fila 4 como borrador" }));
    expect(onAction).toHaveBeenCalledWith(4, "draft");
  });

  it("lets an omitted row back in", async () => {
    const omitted: Decisions = { 3: { row: 3, action: "omit" } };
    const result = { ...REVIEWED, rows: REVIEWED.rows.map((r) => (r.row === 3 ? importRow({ row: 3, status: "skipped", reason: "omitted", name: "RUIZ, BETO" }) : r)) };
    const { user, onAction } = renderPreview(result, { decisions: omitted });
    await openAll(user);
    await user.click(screen.getByRole("button", { name: "Incluir de nuevo la fila 3" }));
    expect(onAction).toHaveBeenCalledWith(3, undefined);
  });

  it("corrects in the row itself: only the flagged fields, prefilled; Enter saves what changed", async () => {
    const { user, onCorrect } = renderPreview(REVIEWED, {});
    await openAll(user);
    await user.click(screen.getByRole("button", { name: "Corregir fila 3" }));
    const editor = screen.getByRole("form", { name: "Corregir fila 3" });
    const phone = within(editor).getByLabelText("Celular");
    expect(phone).toHaveValue("55 12");
    expect(phone).toHaveFocus();
    expect(within(editor).queryByLabelText("CURP")).not.toBeInTheDocument();
    await user.clear(phone);
    await user.type(phone, "55 1234 5678{Enter}");
    expect(onCorrect).toHaveBeenCalledWith(3, { mobilePhone: "55 1234 5678" });
    expect(screen.queryByRole("form", { name: "Corregir fila 3" })).not.toBeInTheDocument();
    // A stable target: the row's header, which the re-preview keeps, and a polite line.
    expect(screen.getByRole("rowheader", { name: "RUIZ, BETO" })).toHaveFocus();
    expect(screen.getByText("Fila 3 corregida")).toBeInTheDocument();
  });

  it("returns focus to Corregir when the correction is cancelled", async () => {
    const { user } = renderPreview(REVIEWED, {});
    await openAll(user);
    await user.click(screen.getByRole("button", { name: "Corregir fila 3" }));
    await user.click(within(screen.getByRole("form", { name: "Corregir fila 3" })).getByRole("button", { name: "Cancelar" }));
    expect(screen.getByRole("button", { name: "Corregir fila 3" })).toHaveFocus();
  });

  it("makes the office choose a state the file named wrong, and keeps one it named right", async () => {
    const rows = [
      importRow({ row: 7, status: "warning", name: "SOTO, CIRO", issues: [{ field: "state", code: "state_unknown", original: "Atlántida", stored: "" }] }),
      importRow({ row: 8, status: "warning", name: "VEGA, EVA", issues: [{ field: "state", code: "state_unknown", original: "Jalisco", stored: "" }] }),
    ];
    const { user } = renderPreview({ ...REVIEWED, rows }, {});
    await openAll(user);
    await user.click(screen.getByRole("button", { name: "Corregir fila 7" }));
    expect(within(screen.getByRole("form", { name: "Corregir fila 7" })).getByLabelText("Estado")).toHaveValue("");
    await user.click(within(screen.getByRole("form", { name: "Corregir fila 7" })).getByRole("button", { name: "Cancelar" }));
    await user.click(screen.getByRole("button", { name: "Corregir fila 8" }));
    expect(within(screen.getByRole("form", { name: "Corregir fila 8" })).getByLabelText("Estado")).toHaveValue("Jalisco");
  });

  it("asks before Esc throws away a changed value", async () => {
    const { user, onCorrect } = renderPreview(REVIEWED, {});
    await openAll(user);
    await user.click(screen.getByRole("button", { name: "Corregir fila 3" }));
    await user.type(screen.getByLabelText("Celular"), "9{Escape}");
    expect(screen.getByRole("form", { name: "Corregir fila 3" })).toBeInTheDocument();
    expect(screen.getByText("¿Descartar cambios?")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Seguir corrigiendo" }));
    expect(screen.getByLabelText("Celular")).toHaveValue("55 129");
    await user.keyboard("{Escape}");
    await user.click(screen.getByRole("button", { name: "Descartar" }));
    expect(onCorrect).not.toHaveBeenCalled();
    expect(screen.queryByRole("form", { name: "Corregir fila 3" })).not.toBeInTheDocument();
  });

  it("checks the format on blur once there is a value, and refuses to save it", async () => {
    const { user, onCorrect } = renderPreview(REVIEWED, {});
    await openAll(user);
    await user.click(screen.getByRole("button", { name: "Corregir fila 3" }));
    const phone = screen.getByLabelText("Celular");
    await user.clear(phone);
    await user.type(phone, "55 99");
    expect(screen.queryByText("Un teléfono lleva 10 dígitos.")).not.toBeInTheDocument();
    await user.tab();
    expect(screen.getByText("Un teléfono lleva 10 dígitos.")).toBeInTheDocument();
    expect(phone).toHaveAttribute("aria-invalid", "true");
    await user.click(screen.getByRole("button", { name: "Guardar corrección" }));
    expect(onCorrect).not.toHaveBeenCalled();
  });

  it("Esc cancels without saving when nothing changed", async () => {
    const { user, onCorrect } = renderPreview(REVIEWED, {});
    await openAll(user);
    await user.click(screen.getByRole("button", { name: "Corregir fila 3" }));
    await user.keyboard("{Escape}");
    expect(onCorrect).not.toHaveBeenCalled();
    expect(screen.queryByRole("form", { name: "Corregir fila 3" })).not.toBeInTheDocument();
  });

  it("shows the server's refusal of a correction in its row and on its field", async () => {
    const refused: RowFieldError[] = [{ row: 3, field: "mobilePhone", code: "phone_invalid" }];
    const { user } = renderPreview(REVIEWED, { rowErrors: refused, decisions: { 3: { row: 3, overrides: { mobilePhone: "55 9" } } } });
    await openAll(user);
    expect(within(rowOf(3) as HTMLElement).getByText("Celular: el teléfono lleva 10 dígitos")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Corregir fila 3" }));
    expect(screen.getByLabelText("Celular")).toHaveValue("55 9");
    expect(screen.getByLabelText("Celular")).toHaveAttribute("aria-invalid", "true");
  });

  it("counts drafts and omitted rows, beside Con avisos", () => {
    const result = { ...REVIEWED, rows: [...REVIEWED.rows, importRow({ row: 5, status: "new", draft: true }), importRow({ row: 6, status: "skipped", reason: "omitted" })] };
    renderPreview(result, {});
    const cards = screen.getByRole("group", { name: "Resumen del archivo" });
    expect(within(cards).getByText("Con avisos")).toBeInTheDocument();
    expect(within(cards).getByText("Borradores").parentElement).toHaveTextContent("1");
    expect(within(cards).getByText("Omitidas").parentElement).toHaveTextContent("1");
  });
});
