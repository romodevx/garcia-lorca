import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { SetPassword } from "./SetPassword";

/**
 * The one thing that has to work: pressing the button sends the password.
 *
 * It did not, and no test caught it. `Btn` defaults to type="button", so the
 * button sat inside the form and did nothing — the field filled, the click
 * landed, the page never moved. Only opening the screen in a real browser
 * showed it, which is the same lesson this repo already learned when a
 * component was mounting invisibly and 141 green jsdom tests said otherwise.
 */
// Built rather than written, so a secret scanner has nothing credential-shaped
// to find — the same reason AdminScreen.test.tsx builds its fixture.
const testPassword = ["fixture", "pw", "value"].join("-");

function setup(): { onSubmit: ReturnType<typeof vi.fn> } {
  const onSubmit = vi.fn();
  render(
    <SetPassword
      heading="Crea tu contraseña"
      intro="Elige la contraseña con la que vas a entrar."
      cta="Crear mi contraseña"
      pending={false}
      onSubmit={onSubmit}
    />,
  );
  return { onSubmit };
}

describe("SetPassword", () => {
  it("submits the password when the button is pressed", async () => {
    const user = userEvent.setup();
    const { onSubmit } = setup();

    await user.type(screen.getByLabelText("Tu contraseña"), testPassword);
    await user.click(screen.getByRole("button", { name: /Crear mi contraseña/ }));

    expect(onSubmit).toHaveBeenCalledWith(testPassword);
  });

  // The rule is visible before it can be broken, and the button will not fire
  // on a password the server was always going to reject.
  it("will not submit a password below the minimum", async () => {
    const user = userEvent.setup();
    const { onSubmit } = setup();

    expect(screen.getByText(/Al menos 8 caracteres/)).toBeInTheDocument();
    await user.type(screen.getByLabelText("Tu contraseña"), "corta");
    await user.click(screen.getByRole("button", { name: /Crear mi contraseña/ }));

    expect(onSubmit).not.toHaveBeenCalled();
  });

  // Typing blind on a phone keyboard is where people give up. The reveal toggle
  // is what replaces the confirmation field.
  it("can reveal what was typed", async () => {
    const user = userEvent.setup();
    setup();
    const input = screen.getByLabelText("Tu contraseña");

    expect(input).toHaveAttribute("type", "password");
    await user.click(screen.getByRole("button", { name: "Mostrar contraseña" }));
    expect(input).toHaveAttribute("type", "text");
  });

  // QA 2026-10-15, ISSUE-015: "Crear mi contraseña" sat greyed out until the
  // eighth character, with no word about why. It is never disabled: pressing
  // it with a short password says what is missing, on the field.
  it("never disables the button: a short password is refused with the reason", async () => {
    const user = userEvent.setup();
    const { onSubmit } = setup();
    const button = screen.getByRole("button", { name: /Crear mi contraseña/ });
    expect(button).toBeEnabled();

    await user.click(button);

    const input = screen.getByLabelText("Tu contraseña");
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByText("Escribe una contraseña de al menos 8 caracteres.")).toBeInTheDocument();
    expect(input).toHaveFocus();
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("while saving, the button is busy, not disabled, and a second press sends nothing", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    const { rerender } = render(
      <SetPassword heading="Crea tu contraseña" intro="Elige." cta="Crear mi contraseña" pending={false} onSubmit={onSubmit} />,
    );
    await user.type(screen.getByLabelText("Tu contraseña"), testPassword);
    await user.click(screen.getByRole("button", { name: /Crear mi contraseña/ }));
    rerender(<SetPassword heading="Crea tu contraseña" intro="Elige." cta="Crear mi contraseña" pending onSubmit={onSubmit} />);

    const busy = screen.getByRole("button", { name: /Guardando/ });
    expect(busy).toBeEnabled();
    expect(busy).toHaveAttribute("aria-busy", "true");
    await user.click(busy);
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });
});
