import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { ErrorSummary } from "./ErrorSummary";
import { TextField } from "./fields";

const noop = (): void => {};

function Form({ attempt, errors }: { attempt: number; errors: Record<string, string> }) {
  return (
    <>
      <ErrorSummary attempt={attempt} items={Object.entries(errors).map(([field, message]) => ({ field, message }))} />
      <TextField label="CURP" value="" onChange={noop} error={errors.CURP} />
      <TextField label="Tutor" value="" onChange={noop} error={errors.Tutor} />
    </>
  );
}

describe("ErrorSummary", () => {
  it("no pinta nada mientras no hay errores", () => {
    render(<Form attempt={0} errors={{}} />);
    expect(screen.queryByRole("heading", { name: /antes de guardar/i })).toBeNull();
  });

  it("al fallar el guardado se lleva el foco y enumera cada problema", () => {
    render(<Form attempt={1} errors={{ CURP: "Escribe el CURP del alumno.", Tutor: "Escribe el nombre del tutor." }} />);
    const summary = screen.getByRole("group", { name: /antes de guardar/i });
    expect(summary).toHaveFocus();
    expect(screen.getByRole("link", { name: "CURP: Escribe el CURP del alumno." })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Tutor: Escribe el nombre del tutor." })).toBeInTheDocument();
  });

  it("cada renglón lleva al campo", async () => {
    render(<Form attempt={1} errors={{ CURP: "Escribe el CURP.", Tutor: "Escribe el tutor." }} />);
    await userEvent.click(screen.getByRole("link", { name: /^Tutor:/ }));
    expect(screen.getByLabelText("Tutor")).toHaveFocus();
  });

  it("un segundo intento con los mismos errores vuelve a llevarse el foco", () => {
    const errors = { CURP: "Escribe el CURP." };
    const { rerender } = render(<Form attempt={1} errors={errors} />);
    screen.getByLabelText("Tutor").focus();
    rerender(<Form attempt={2} errors={errors} />);
    expect(screen.getByRole("group", { name: /antes de guardar/i })).toHaveFocus();
  });
});
