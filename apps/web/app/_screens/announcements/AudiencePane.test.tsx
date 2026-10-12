import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { AudiencePane } from "./AudiencePane";

describe("AudiencePane", () => {
  it("is the Destinatarios column, with the four audiences in order and the chosen one pressed", () => {
    render(<AudiencePane scope="level" onChooseScope={() => {}} pickedCount={0}><p>lista</p></AudiencePane>);
    const pane = screen.getByRole("region", { name: "Destinatarios" });
    const rows = screen.getAllByRole("button").map((b) => [b.textContent, b.getAttribute("aria-pressed")]);
    expect(rows).toEqual([["Un alumno", "false"], ["Grupos", "false"], ["Niveles", "true"], ["Toda la escuela", "false"]]);
    expect(pane).toHaveTextContent("lista");
  });

  it("asks the composer to change the audience", async () => {
    const onChooseScope = vi.fn();
    render(<AudiencePane scope="group" onChooseScope={onChooseScope} pickedCount={0}>{null}</AudiencePane>);
    await userEvent.setup().click(screen.getByRole("button", { name: "Toda la escuela" }));
    expect(onChooseScope).toHaveBeenCalledWith("cycle");
  });

  // B6: the counter only once there is more than one.
  it("counts the picks at its foot once there is more than one", () => {
    const { rerender } = render(<AudiencePane scope="group" onChooseScope={() => {}} pickedCount={1}>{null}</AudiencePane>);
    expect(screen.queryByText(/seleccionados/)).not.toBeInTheDocument();
    rerender(<AudiencePane scope="group" onChooseScope={() => {}} pickedCount={3}>{null}</AudiencePane>);
    expect(screen.getByText("3 seleccionados")).toBeInTheDocument();
  });
});
