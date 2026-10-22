import { render, screen } from "@testing-library/react";
import type { ReactElement } from "react";
import { describe, expect, it } from "vitest";

import { Field } from "./Field";
import { TextField } from "./fields";
import { DateField } from "./DateField";

const child = (id: string, describedBy: string | undefined): ReactElement => (
  <input id={id} aria-describedby={describedBy} />
);

const WARNING = "En el Excel decía «55 12»; no se guardó.";

describe("Field — the warning variant (PLAN-import-avisos)", () => {
  it("shows the warning politely and ties it to the field", () => {
    render(<Field label="Teléfono" warning={WARNING}>{child}</Field>);
    const input = screen.getByLabelText("Teléfono");
    const msg = screen.getByText(WARNING);
    expect(msg.closest("[role=status]")).not.toBeNull();
    expect(input.getAttribute("aria-describedby")).toContain(msg.id);
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("does not mark the field invalid: a warning never blocks", () => {
    render(<TextField label="Teléfono" value="" onChange={() => undefined} warning={WARNING} />);
    expect(screen.getByLabelText("Teléfono")).not.toHaveAttribute("aria-invalid");
  });

  it("lets the error win: only the error is read and shown", () => {
    render(<TextField label="Teléfono" value="" onChange={() => undefined} warning={WARNING} error="Un teléfono lleva 10 dígitos." />);
    const input = screen.getByLabelText("Teléfono");
    expect(screen.queryByText(WARNING)).toBeNull();
    expect(input.getAttribute("aria-describedby")).toBe(screen.getByText("Un teléfono lleva 10 dígitos.").id);
  });

  it("keeps the help reachable beside the warning", () => {
    render(<Field label="Teléfono" warning={WARNING} help="Diez dígitos.">{child}</Field>);
    const describedBy = screen.getByLabelText("Teléfono").getAttribute("aria-describedby") ?? "";
    expect(describedBy.split(" ")).toHaveLength(2);
  });

  it("renders the warning's action after it, outside the described text", () => {
    render(
      <Field label="Teléfono" warning={WARNING} warningAction={<button type="button">Está bien así</button>}>
        {child}
      </Field>,
    );
    const msg = screen.getByText(WARNING);
    expect(msg).not.toHaveTextContent("Está bien así");
    expect(screen.getByRole("button", { name: "Está bien así" })).toBeInTheDocument();
  });

  it("reaches DateField too", () => {
    render(<DateField label="Fecha de nacimiento" value="" onChange={() => undefined} warning={WARNING} />);
    expect(screen.getByLabelText("Fecha de nacimiento").getAttribute("aria-describedby")).toBe(screen.getByText(WARNING).id);
  });

  it("without a warning nothing changes", () => {
    render(<Field label="Teléfono">{child}</Field>);
    expect(screen.queryByRole("status")).toBeNull();
    expect(screen.getByLabelText("Teléfono")).not.toHaveAttribute("aria-describedby");
  });
});
