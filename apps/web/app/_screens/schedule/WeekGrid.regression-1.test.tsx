// Regression: ISSUE-001/002 — an uncovered class: "Sin cubrir" was clipped in a
// 50-minute block, and toggling it mixed `border` and `borderLeft` (React warns).
// Found by /qa on 2026-10-02
// Report: .gstack/qa-reports/qa-report-localhost-2026-10-02.md
import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { sampleBoard } from "../../schedule/__fixtures__/board";
import { indexBoard } from "../../schedule/catalog";
import { WeekGrid } from "./WeekGrid";

afterEach(() => vi.restoreAllMocks());

describe("WeekGrid — an uncovered class (T19)", () => {
  it("says «Sin cubrir» on the time line and re-renders without mixing border shorthands", () => {
    const board = sampleBoard();
    const index = indexBoard(board);
    const block = board.blocks[0]!;
    const errors = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const grid = { start: 7 * 60, end: 15 * 60 };
    const { rerender } = render(<WeekGrid blocks={[block]} index={index} grid={grid} onOpen={() => undefined} uncovered={new Set()} />);
    rerender(<WeekGrid blocks={[block]} index={index} grid={grid} onOpen={() => undefined} uncovered={new Set([block.id])} />);
    expect(screen.getByText("08:00–08:50 · Sin cubrir")).toBeInTheDocument();
    expect(errors.mock.calls.filter((c) => String(c[0]).includes("style property"))).toEqual([]);
  });
});
