import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as importApi from "../../studentImport/api";
import type { StudentImportFamilies } from "../../studentImport/api";
import { ImportFamilies } from "./ImportFamilies";

vi.mock("../../studentImport/api", async (importOriginal) => ({
  ...(await importOriginal<typeof importApi>()),
  undoStudentImportFamilies: vi.fn(),
}));
const api = vi.mocked(importApi);

afterEach(() => vi.clearAllMocks());

const fam = (id: string, name?: string) => ({ id, ...(name === undefined ? {} : { name }) });
const built = (over: Partial<StudentImportFamilies> = {}): StudentImportFamilies => ({ built: true, created: [], joined: [], skipped: 0, ...over });

function renderBlock(families: StudentImportFamilies, onReview = vi.fn()) {
  render(<ImportFamilies families={families} batchId="b1" review={{ href: "/alumnos?vista=familias", onOpen: onReview }} />);
  return onReview;
}

describe("the import's Familias block", () => {
  it("says how many families it built and offers Deshacer", () => {
    renderBlock(built({ created: [fam("f1", "GARCÍA TORRES"), fam("f2", "RUIZ PAZ")], joined: [fam("f3")] }));
    const block = screen.getByRole("region", { name: "Familias" });
    expect(block).toHaveTextContent("Armamos 2 familias automáticamente y sumamos alumnos a 1 familia que ya existía.");
    expect(screen.getByRole("button", { name: "Deshacer" })).toBeInTheDocument();
    expect(screen.getByText("GARCÍA TORRES")).toBeInTheDocument();
  });

  it("collapses a long list of names behind «Ver familias»", () => {
    const many = ["A", "B", "C", "D", "E", "F"].map((n) => fam(n, `FAMILIA ${n}`));
    renderBlock(built({ created: many }));
    expect(screen.getByText("Ver familias")).toBeInTheDocument();
    expect(screen.getByText("Ver familias").closest("details")).not.toHaveAttribute("open");
  });

  it("links to the Familias tab when suggestions are left to review", async () => {
    const user = userEvent.setup();
    const onReview = renderBlock(built({ created: [fam("f1")], toReview: 3 }));
    const link = screen.getByRole("link", { name: /3 posibles familias por revisar/ });
    expect(link).toHaveAttribute("href", "/alumnos?vista=familias");
    await user.click(link);
    expect(onReview).toHaveBeenCalledTimes(1);
  });

  it("says when nothing was very likely, with no undo", () => {
    renderBlock(built({ toReview: 1 }));
    expect(screen.getByText("No encontramos familias muy probables.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Deshacer" })).toBeNull();
    expect(screen.getByRole("link", { name: /1 posible familia por revisar/ })).toBeInTheDocument();
  });

  it("an incomplete build is unconfirmed and still offers Deshacer, even with nothing listed", () => {
    renderBlock(built({ built: false }));
    expect(screen.getByText("No pudimos confirmar todas las familias; lo que no se armó queda como sugerencia.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Deshacer" })).toBeInTheDocument();
  });

  it("undoes without asking, ignores a second press while busy, and keeps the outcome", async () => {
    const user = userEvent.setup();
    let answer: (v: importApi.StudentImportFamiliesUndo) => void = () => undefined;
    api.undoStudentImportFamilies.mockImplementation(() => new Promise((resolve) => { answer = resolve; }));
    renderBlock(built({ created: [fam("f1"), fam("f2")] }));

    const undo = screen.getByRole("button", { name: "Deshacer" });
    await user.click(undo);
    await user.click(undo);
    expect(undo).not.toBeDisabled();
    expect(undo).toHaveAttribute("aria-busy", "true");
    expect(api.undoStudentImportFamilies).toHaveBeenCalledTimes(1);
    expect(api.undoStudentImportFamilies).toHaveBeenCalledWith(expect.any(String), "b1");
    expect(screen.queryByRole("dialog")).toBeNull();

    answer({ removed: 4, dissolved: 2, keptWithAccess: 1 });
    const status = await screen.findByRole("status");
    expect(status).toHaveTextContent("Deshicimos 2 familias.");
    expect(status).toHaveTextContent("1 familia ya tiene acceso a la app y no se tocó; quita el acceso desde la familia primero.");
  });

  it("an undo that fails says so inline", async () => {
    const user = userEvent.setup();
    api.undoStudentImportFamilies.mockRejectedValue(new importApi.StudentImportError(500, "boom", undefined));
    renderBlock(built({ created: [fam("f1")] }));
    await user.click(screen.getByRole("button", { name: "Deshacer" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("No pudimos deshacer las familias. Intenta de nuevo.");
    await waitFor(() => expect(screen.getByRole("button", { name: "Deshacer" })).not.toHaveAttribute("aria-busy"));
  });
});
