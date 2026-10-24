import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as importApi from "../../studentImport/api";
import type { StudentImportFamilies } from "../../studentImport/api";
import { ImportFamilies, type FamiliesReviewLink } from "./ImportFamilies";

vi.mock("../../studentImport/api", async (importOriginal) => ({
  ...(await importOriginal<typeof importApi>()),
  undoStudentImportFamilies: vi.fn(),
}));
const api = vi.mocked(importApi);

afterEach(() => vi.clearAllMocks());

const fam = (id: string, name?: string) => ({ id, ...(name === undefined ? {} : { name }) });
const built = (over: Partial<StudentImportFamilies> = {}): StudentImportFamilies => ({ built: true, created: [], joined: [], skipped: 0, ...over });

/** `withoutBatch` because a default parameter would swallow an explicit undefined. */
function renderBlock(families: StudentImportFamilies, { withoutBatch = false } = {}): FamiliesReviewLink {
  const batchId = withoutBatch ? undefined : "b1";
  const review: FamiliesReviewLink = {
    href: "/alumnos?vista=familias",
    onOpen: vi.fn(),
    familyHref: (id) => `/alumnos?vista=familias&familia=${id}`,
    onOpenFamily: vi.fn(),
  };
  render(<ImportFamilies families={families} batchId={batchId} review={review} />);
  return review;
}

describe("the import's Familias block", () => {
  it("says how many families it built and offers Deshacer", () => {
    renderBlock(built({ created: [fam("f1", "GARCÍA TORRES"), fam("f2", "RUIZ PAZ")], joined: [fam("f3")] }));
    const block = screen.getByRole("region", { name: "Familias" });
    expect(block).toHaveTextContent("Armamos 2 familias automáticamente y sumamos alumnos a 1 familia que ya existía.");
    expect(screen.getByRole("button", { name: "Deshacer" })).toBeInTheDocument();
  });

  it("links each named family to its page in Alumnos › Familias", async () => {
    const user = userEvent.setup();
    const review = renderBlock(built({ created: [fam("f1", "GARCÍA TORRES")] }));
    const link = screen.getByRole("link", { name: "GARCÍA TORRES" });
    expect(link).toHaveAttribute("href", "/alumnos?vista=familias&familia=f1");
    await user.click(link);
    expect(review.onOpenFamily).toHaveBeenCalledWith("f1");
  });

  it("collapses a long list of names behind «Ver familias»", () => {
    const many = ["A", "B", "C", "D", "E", "F"].map((n) => fam(n, `FAMILIA ${n}`));
    renderBlock(built({ created: many }));
    const group = screen.getByRole("group");
    expect(group).not.toHaveAttribute("open");
    expect(within(group).getByText("Ver familias")).toBeInTheDocument();
  });

  it("links to the Familias tab when suggestions are left to review", async () => {
    const user = userEvent.setup();
    const review = renderBlock(built({ created: [fam("f1")], toReview: 3 }));
    const link = screen.getByRole("link", { name: /3 posibles familias por revisar/ });
    expect(link).toHaveAttribute("href", "/alumnos?vista=familias");
    await user.click(link);
    expect(review.onOpen).toHaveBeenCalledTimes(1);
  });

  it.each([
    ["absent (no students:read)", undefined],
    ["zero", 0],
  ])("no review link when toReview is %s", (_label, toReview) => {
    renderBlock(built({ created: [fam("f1")], ...(toReview === undefined ? {} : { toReview }) }));
    expect(screen.queryByRole("link", { name: /por revisar/ })).toBeNull();
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

  it("no batch to undo against: no Deshacer", () => {
    renderBlock(built({ created: [fam("f1")] }), { withoutBatch: true });
    expect(screen.queryByRole("button", { name: "Deshacer" })).toBeNull();
  });

  it("undoes without asking, ignores a second press while busy, and keeps the outcome", async () => {
    const user = userEvent.setup();
    let answer: (v: importApi.StudentImportFamiliesUndo) => void = () => undefined;
    api.undoStudentImportFamilies.mockImplementation(() => new Promise((resolve) => { answer = resolve; }));
    renderBlock(built({ created: [fam("f1", "GARCÍA TORRES"), fam("f2")] }));

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
    expect(status).toHaveTextContent("Sacamos a 4 alumnos de sus familias y deshicimos 2 familias que quedaron vacías.");
    expect(status).toHaveTextContent("1 familia ya tiene acceso a la app y no se tocó: para deshacerla, quita el acceso y saca a los alumnos desde Alumnos › Familias.");
    // Something was kept: the headline and the way to try again stay.
    expect(screen.getByText("Armamos 2 familias automáticamente.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Deshacer" })).toBeInTheDocument();
  });

  it("a complete undo changes the headline and retires the names and the button", async () => {
    const user = userEvent.setup();
    api.undoStudentImportFamilies.mockResolvedValue({ removed: 2, dissolved: 1, keptWithAccess: 0 });
    renderBlock(built({ created: [fam("f1", "GARCÍA TORRES")], toReview: 2 }));
    await user.click(screen.getByRole("button", { name: "Deshacer" }));

    expect(await screen.findByText("Deshicimos las familias que armamos.")).toBeInTheDocument();
    expect(screen.queryByText(/Armamos/)).toBeNull();
    expect(screen.queryByRole("link", { name: "GARCÍA TORRES" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Deshacer" })).toBeNull();
    expect(screen.getByRole("status")).toHaveTextContent("Sacamos a 2 alumnos de sus familias y deshicimos 1 familia que quedó vacía.");
    expect(screen.getByRole("link", { name: /2 posibles familias por revisar/ })).toBeInTheDocument();
  });

  it.each([
    [429, "Demasiados intentos seguidos. Espera un momento y vuelve a intentar."],
    [403, "Tu cuenta no tiene permiso para deshacer familias."],
    [500, "No pudimos deshacer las familias. Intenta de nuevo."],
  ])("an undo refused with %i says so inline", async (status, line) => {
    const user = userEvent.setup();
    api.undoStudentImportFamilies.mockRejectedValue(new importApi.StudentImportError(status, "x", undefined));
    renderBlock(built({ created: [fam("f1")] }));
    await user.click(screen.getByRole("button", { name: "Deshacer" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(line);
    await waitFor(() => expect(screen.getByRole("button", { name: "Deshacer" })).not.toHaveAttribute("aria-busy"));
  });
});
