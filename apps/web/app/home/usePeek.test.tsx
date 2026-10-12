import { fireEvent, render, screen } from "@testing-library/react";
import type { ReactElement } from "react";
import { describe, expect, it } from "vitest";

import { moveFocusAmongShortcuts } from "./usePeek";

function Shortcuts(): ReactElement {
  return (
    <ul data-testid="peek" onKeyDown={moveFocusAmongShortcuts}>
      {["Uno", "Dos", "Tres"].map((label) => (
        <li key={label}>
          <button type="button">{label}</button>
        </li>
      ))}
    </ul>
  );
}

// Review M4: an arrow pressed while no shortcut has the focus yet (the list
// itself, or something outside it) starts from an end, not from the middle.
describe("moveFocusAmongShortcuts with no shortcut focused", () => {
  it.each([
    ["ArrowUp", "Tres"],
    ["ArrowLeft", "Tres"],
    ["ArrowDown", "Uno"],
    ["ArrowRight", "Uno"],
  ])("%s goes to %s", (key, expected) => {
    render(<Shortcuts />);
    fireEvent.keyDown(screen.getByTestId("peek"), { key });
    expect(screen.getByRole("button", { name: expected })).toHaveFocus();
  });
});
