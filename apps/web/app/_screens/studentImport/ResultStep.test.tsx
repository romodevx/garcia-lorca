import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { ResultStep } from "./ResultStep";
import { ALREADY_TO_MOVE, MATCHED } from "./fixtures";

function renderResult(result: Parameters<typeof ResultStep>[0]["result"]) {
  render(<ResultStep result={result} cut={undefined} onAnother={vi.fn()} onViewStudents={vi.fn()} />);
}

describe("the import's result", () => {
  it("says how many enrollments moved to the cycle start (T14)", () => {
    renderResult(ALREADY_TO_MOVE);
    expect(screen.getByText("2 inscripciones movidas al inicio del ciclo.")).toBeInTheDocument();
  });

  it("says nothing about moves when there were none", () => {
    renderResult(MATCHED);
    expect(screen.queryByText(/movidas? al inicio del ciclo/)).not.toBeInTheDocument();
  });
});

describe("the import's result — Familias (PLAN-familias-importacion I3)", () => {
  const WITH_FAMILIES = { ...MATCHED, batchId: "b1", families: { built: true, created: [{ id: "f1", name: "GARCÍA TORRES" }], joined: [], skipped: 0 } };
  const REVIEW = { href: "/alumnos?vista=familias", onOpen: vi.fn(), familyHref: (id: string) => `/alumnos?vista=familias&familia=${id}`, onOpenFamily: vi.fn() };

  it("module_families on: shows the families the commit built", () => {
    render(<ResultStep result={WITH_FAMILIES} cut={undefined} onAnother={vi.fn()} onViewStudents={vi.fn()} familiesReview={REVIEW} />);
    expect(screen.getByRole("region", { name: "Familias" })).toHaveTextContent("Armamos 1 familia automáticamente.");
  });

  it("module_families off: no block, even if the answer carried one", () => {
    renderResult(WITH_FAMILIES);
    expect(screen.queryByRole("region", { name: "Familias" })).toBeNull();
  });

  it("no families in the answer: no block", () => {
    render(<ResultStep result={MATCHED} cut={undefined} onAnother={vi.fn()} onViewStudents={vi.fn()} familiesReview={REVIEW} />);
    expect(screen.queryByRole("region", { name: "Familias" })).toBeNull();
  });
});
