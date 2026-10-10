import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { GradeLevel, Level } from "../../../config/api";
import { LevelGradeChips } from "./LevelGradeChips";

const PRIMARIA: Level = { id: "l1", name: "PRIMARIA", sepCode: "", enrollmentGranularity: "cycle" };
const SECUNDARIA: Level = { id: "l2", name: "SECUNDARIA", sepCode: "", enrollmentGranularity: "cycle" };
const GRADES: GradeLevel[] = [
  { id: "g2", levelId: "l1", name: "2° PRIMARIA", ordinal: 2 },
  { id: "g1", levelId: "l1", name: "1° PRIMARIA", ordinal: 1 },
];

describe("LevelGradeChips", () => {
  it("picks a level from the shared selection", async () => {
    const onSelectLevel = vi.fn();
    const user = userEvent.setup();
    render(<LevelGradeChips levels={[PRIMARIA, SECUNDARIA]} selectedLevelId="l1" onSelectLevel={onSelectLevel} />);

    const levels = screen.getByRole("group", { name: "Nivel" });
    expect(within(levels).getByRole("button", { name: "PRIMARIA" })).toHaveAttribute("aria-pressed", "true");
    await user.click(within(levels).getByRole("button", { name: "SECUNDARIA" }));
    expect(onSelectLevel).toHaveBeenCalledWith("l2");
    expect(screen.queryByRole("group", { name: "Grado" })).toBeNull();
  });

  it("offers the selected level's grades in order, each with its pill", async () => {
    const onSelectGrade = vi.fn();
    const user = userEvent.setup();
    render(
      <LevelGradeChips
        levels={[PRIMARIA]}
        selectedLevelId="l1"
        onSelectLevel={() => {}}
        grades={GRADES}
        selectedGradeId="g1"
        onSelectGrade={onSelectGrade}
        gradePill={(id) => (id === "g1" ? "1 grupo" : "sin grupos")}
      />,
    );

    const grades = within(screen.getByRole("group", { name: "Grado" })).getAllByRole("button");
    // QA: the name read "1ºsin grupos"; a screen reader needs the pause between grade and count.
    expect(grades.map((g) => g.textContent)).toEqual(["1° PRIMARIA, 1 grupo", "2° PRIMARIA, sin grupos"]);
    expect(screen.getByRole("button", { name: "1° PRIMARIA, 1 grupo" })).toBeInTheDocument();
    expect(grades[0]).toHaveAttribute("aria-pressed", "true");
    await user.click(grades[1] as HTMLElement);
    expect(onSelectGrade).toHaveBeenCalledWith("g2");
  });

  it("leaves the pills out when there is nothing to count yet", () => {
    render(<LevelGradeChips levels={[PRIMARIA]} selectedLevelId="l1" onSelectLevel={() => {}} grades={GRADES} onSelectGrade={() => {}} />);
    expect(screen.getByRole("button", { name: "1° PRIMARIA" })).toBeInTheDocument();
  });
});
