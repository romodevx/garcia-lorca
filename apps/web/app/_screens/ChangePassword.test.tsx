import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { HttpError } from "../auth/httpError";
import { Providers } from "../test-utils";
import { ChangePassword } from "./ChangePassword";

/*
 * The FORCED change (temporary password). It shares the at-will dialog's
 * logic: the primary is never disabled, errors appear on submit, and a 204
 * (password changed, session not renewed) hands over to sign-in.
 */

type Submit = (current: string, next: string) => Promise<{ sessionRenewed: boolean }>;

function renderScreen(onSubmit: Submit) {
  const onDone = vi.fn();
  const onSessionEnded = vi.fn();
  render(
    <Providers>
      <ChangePassword theme="light" onToggle={() => {}} onSubmit={onSubmit} onDone={onDone} onSessionEnded={onSessionEnded} />
    </Providers>
  );
  return { onDone, onSessionEnded };
}

async function fillValid(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.type(screen.getByLabelText("Contraseña temporal"), "temporal-1");
  await user.type(screen.getByLabelText("Nueva contraseña"), "nueva-larga-1");
  await user.type(screen.getByLabelText("Confirmar contraseña"), "nueva-larga-1");
}

const primary = (): HTMLElement => screen.getByRole("button", { name: "Guardar y entrar" });

describe("ChangePassword (forced)", () => {
  it("never disables the primary; an empty submit says what is missing and focuses it", async () => {
    const onSubmit = vi.fn<Submit>();
    const user = userEvent.setup();
    renderScreen(onSubmit);
    expect(primary()).toBeEnabled();
    await user.click(primary());
    expect(screen.getByText("Escribe tu contraseña actual.")).toBeInTheDocument();
    expect(screen.getByLabelText("Contraseña temporal")).toHaveFocus();
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("ignores a repeat click while saving, then reports a renewed session", async () => {
    let finish: (v: { sessionRenewed: boolean }) => void = () => {};
    const onSubmit = vi.fn<Submit>().mockImplementation(() => new Promise((resolve) => { finish = resolve; }));
    const user = userEvent.setup();
    const { onDone } = renderScreen(onSubmit);
    await fillValid(user);
    const button = primary();
    await user.click(button);
    expect(button).toBeEnabled();
    expect(button).toHaveAttribute("aria-busy", "true");
    await user.click(button);
    expect(onSubmit).toHaveBeenCalledTimes(1);
    finish({ sessionRenewed: true });
    await vi.waitFor(() => expect(onDone).toHaveBeenCalled());
  });

  it("hands over to sign-in on a 204 (session not renewed)", async () => {
    const user = userEvent.setup();
    const { onDone, onSessionEnded } = renderScreen(vi.fn<Submit>().mockResolvedValue({ sessionRenewed: false }));
    await fillValid(user);
    await user.click(primary());
    await vi.waitFor(() => expect(onSessionEnded).toHaveBeenCalled());
    expect(onDone).not.toHaveBeenCalled();
  });

  it("maps a 401 to the temporary-password field", async () => {
    const user = userEvent.setup();
    renderScreen(vi.fn<Submit>().mockRejectedValue(new HttpError(401, "changePassword: HTTP 401")));
    await fillValid(user);
    await user.click(primary());
    expect(await screen.findByText("La contraseña actual no es correcta.")).toBeInTheDocument();
    expect(screen.getByLabelText("Contraseña temporal")).toHaveFocus();
  });
});
