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
