import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { PupilChips } from "./PupilChips";

describe("PupilChips", () => {
  it("each chip's way out is a 44 × 44 target", () => {
    render(<PupilChips pupils={[{ id: "s1", firstName: "", lastName: "", name: "ANA", room: "A", admissionStatus: "enrolled" }]} onRemove={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Quitar a ANA" })).toHaveStyle({ width: "44px", height: "44px" });
  });
});
