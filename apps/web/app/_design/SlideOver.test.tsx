import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { SlideOver } from "./SlideOver";

function open(props: Partial<React.ComponentProps<typeof SlideOver>> = {}) {
  return render(
    <SlideOver
      title="Nuevo nivel"
      subtitle="Alta rápida"
      icon="settings"
      onClose={props.onClose ?? (() => {})}
      onSubmit={props.onSubmit ?? (() => {})}
      submitLabel="Crear nivel"
      {...props}
    >
      <label>
        Nombre
        <input aria-label="Nombre" />
      </label>
    </SlideOver>,
  );
}

describe("SlideOver", () => {
  it("names itself, so a person and a test find the same thing", () => {
    open();
    expect(screen.getByRole("dialog", { name: "Nuevo nivel" })).toBeInTheDocument();
    expect(screen.getByLabelText("Nombre")).toBeInTheDocument();
  });

  // Enter saves. A form that only submits by mouse is a form somebody fills in
  // slower than the paper it replaced.
  it("saves on Enter", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    open({ onSubmit });

    await user.type(screen.getByLabelText("Nombre"), "Primaria{Enter}");

    expect(onSubmit).toHaveBeenCalled();
  });

  it("closes on Escape and on the backdrop", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    open({ onClose });

    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledTimes(1);

    await user.click(screen.getByTestId("slideover-backdrop"));
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  // Clicking inside must not close it. Losing a half-typed form to a stray
  // click is the fastest way to make somebody stop using a screen.
  it("does not close when the panel itself is clicked", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    open({ onClose });

    await user.click(screen.getByLabelText("Nombre"));

    expect(onClose).not.toHaveBeenCalled();
  });

  // While saving, the button says so and cannot be pressed twice — one click,
  // one row.
  it("shows the save in flight and refuses a second press", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    open({ onSubmit, pending: true });

    const save = screen.getByRole("button", { name: /Guardando/ });
    await user.click(save);

    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("shows the reason a save was refused", () => {
    open({ error: "Ya existe un nivel con ese nombre." });
    expect(screen.getByRole("alert")).toHaveTextContent("Ya existe un nivel con ese nombre.");
  });
});
