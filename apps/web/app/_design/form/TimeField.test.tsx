import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { TimeField } from "./TimeField";

describe("TimeField (ISSUE-028)", () => {
  it("is a time input the user can type into, bounded and stepped", () => {
    render(<TimeField label="Empieza" value="07:00" onChange={vi.fn()} min="07:00" max="20:00" stepMinutes={5} />);
    const input = screen.getByLabelText("Empieza");
    expect(input).toHaveAttribute("type", "time");
    expect(input).toHaveAttribute("min", "07:00");
    expect(input).toHaveAttribute("max", "20:00");
    expect(input).toHaveAttribute("step", "300");
  });

  it("shows a saved time outside its bounds instead of clearing it", () => {
    render(<TimeField label="Empieza" value="06:30" onChange={vi.fn()} min="07:00" max="20:00" />);
    expect(screen.getByLabelText("Empieza")).toHaveValue("06:30");
  });

  it("hands over what is typed and reports the blur", async () => {
    const onChange = vi.fn();
    const onBlur = vi.fn();
    const user = userEvent.setup();
    render(<TimeField label="Termina" value="" onChange={onChange} onBlur={onBlur} />);
    const input = screen.getByLabelText("Termina");
    // jsdom has no hour/minute segments: a typed time arrives as one change.
    await user.click(input);
    fireEvent.change(input, { target: { value: "08:30" } });
    await user.tab();
    expect(onChange).toHaveBeenLastCalledWith("08:30");
    expect(onBlur).toHaveBeenCalled();
  });

  it("wires its error to the input", () => {
    render(<TimeField label="Termina" value="21:00" onChange={vi.fn()} error="Escribe una hora entre 07:00 y 20:00." />);
    const input = screen.getByLabelText("Termina");
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(input).toHaveAccessibleDescription("Escribe una hora entre 07:00 y 20:00.");
  });
});
