import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { HttpError } from "../../auth/httpError";
import { Providers } from "../../test-utils";
import { ChangePasswordDialog } from "./ChangePasswordDialog";

/*
 * ISSUE-T14-103: nobody could change their own password at will. Three fields,
 * so a small dialog (ui-forms-and-modals: container by field count). The
 * primary is never disabled; errors appear on submit and focus moves to the
 * first field that needs attention.
 */

type Submit = (current: string, next: string) => Promise<{ sessionRenewed: boolean }>;

function renderDialog(onSubmit: Submit, extra: { onClose?: () => void; onSessionEnded?: () => void } = {}) {
  const onClose = extra.onClose ?? vi.fn();
  const onSessionEnded = extra.onSessionEnded ?? vi.fn();
  render(
    <Providers>
      <ChangePasswordDialog onSubmit={onSubmit} onClose={onClose} onSessionEnded={onSessionEnded} />
    </Providers>
  );
  return { onClose, onSessionEnded };
}

async function fill(user: ReturnType<typeof userEvent.setup>, current: string, next: string, confirm: string): Promise<void> {
  if (current) await user.type(screen.getByLabelText("Contraseña actual"), current);
  if (next) await user.type(screen.getByLabelText("Nueva contraseña"), next);
  if (confirm) await user.type(screen.getByLabelText("Confirmar contraseña"), confirm);
}

const save = (): HTMLElement => screen.getByRole("button", { name: /Cambiar contraseña/ });

describe("ChangePasswordDialog", () => {
  it("is a labelled dialog that starts on the current password, with an enabled primary", async () => {
    renderDialog(vi.fn());
    expect(await screen.findByRole("dialog", { name: "Cambiar contraseña" })).toBeInTheDocument();
    expect(screen.getByLabelText("Contraseña actual")).toHaveAttribute("autocomplete", "current-password");
    expect(screen.getByLabelText("Nueva contraseña")).toHaveAttribute("autocomplete", "new-password");
    expect(save()).toBeEnabled();
  });

  it("on an empty submit says what is missing and focuses the first field, without calling the server", async () => {
    const onSubmit = vi.fn();
    const user = userEvent.setup();
    renderDialog(onSubmit);
    await screen.findByRole("dialog");
    await user.click(save());
    expect(screen.getByText("Escribe tu contraseña actual.")).toBeInTheDocument();
    expect(screen.getByLabelText("Contraseña actual")).toHaveFocus();
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("checks length and confirmation on submit and focuses the offending field", async () => {
    const onSubmit = vi.fn();
    const user = userEvent.setup();
    renderDialog(onSubmit);
    await screen.findByRole("dialog");
    await fill(user, "actual-123", "corta", "corta");
    await user.click(save());
    expect(screen.getByText("Usa al menos 8 caracteres.")).toBeInTheDocument();
    expect(screen.getByLabelText("Nueva contraseña")).toHaveFocus();

    await user.clear(screen.getByLabelText("Nueva contraseña"));
    await user.type(screen.getByLabelText("Nueva contraseña"), "nueva-larga-1");
    await user.click(save());
    expect(screen.getByText("No coincide con la nueva contraseña.")).toBeInTheDocument();
    expect(screen.getByLabelText("Confirmar contraseña")).toHaveFocus();
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("maps a 401 to the current-password field", async () => {
    const onSubmit = vi.fn<Submit>().mockRejectedValue(new HttpError(401, "changePassword: HTTP 401"));
    const user = userEvent.setup();
    renderDialog(onSubmit);
    await screen.findByRole("dialog");
    await fill(user, "equivocada", "nueva-larga-1", "nueva-larga-1");
    await user.click(save());
    expect(await screen.findByText("La contraseña actual no es correcta.")).toBeInTheDocument();
    expect(screen.getByLabelText("Contraseña actual")).toHaveFocus();
    expect(onSubmit).toHaveBeenCalledWith("equivocada", "nueva-larga-1");
  });

  it.each([
    [new HttpError(400, "changePassword: HTTP 400"), "La nueva contraseña no cumple los requisitos: usa al menos 8 caracteres."],
    [new TypeError("Failed to fetch"), "No pudimos contactar al servidor. Revisa tu conexión e intenta de nuevo."],
    [new HttpError(500, "changePassword: HTTP 500"), "No se pudo cambiar la contraseña. Intenta de nuevo en un momento."],
  ])("explains a failed change in words (%s)", async (err, message) => {
    const user = userEvent.setup();
    renderDialog(vi.fn<Submit>().mockRejectedValue(err));
    await screen.findByRole("dialog");
    await fill(user, "actual-123", "nueva-larga-1", "nueva-larga-1");
    await user.click(save());
    expect(await screen.findByRole("alert")).toHaveTextContent(message);
  });

  it("ignores a second click while saving instead of disabling the button", async () => {
    let finish: (v: { sessionRenewed: boolean }) => void = () => {};
    const onSubmit = vi.fn<Submit>().mockImplementation(() => new Promise((resolve) => { finish = resolve; }));
    const user = userEvent.setup();
    renderDialog(onSubmit);
    await screen.findByRole("dialog");
    await fill(user, "actual-123", "nueva-larga-1", "nueva-larga-1");
    const primary = save();
    await user.click(primary);
    expect(primary).toBeEnabled();
    expect(primary).toHaveAttribute("aria-busy", "true");
    await user.click(primary);
    expect(onSubmit).toHaveBeenCalledTimes(1);
    finish({ sessionRenewed: true });
    expect(await screen.findByRole("status")).toHaveTextContent("Tu contraseña cambió");
  });

  it("confirms a renewed session and closes on Listo", async () => {
    const user = userEvent.setup();
    const { onClose, onSessionEnded } = renderDialog(vi.fn<Submit>().mockResolvedValue({ sessionRenewed: true }));
    await screen.findByRole("dialog");
    await fill(user, "actual-123", "nueva-larga-1", "nueva-larga-1");
    await user.keyboard("{Enter}");
    expect(await screen.findByRole("status")).toHaveTextContent("Tu contraseña cambió");
    await user.click(screen.getByRole("button", { name: "Listo" }));
    expect(onClose).toHaveBeenCalled();
    expect(onSessionEnded).not.toHaveBeenCalled();
  });

  it("hands over to sign-in when the server could not renew the session", async () => {
    const user = userEvent.setup();
    const { onSessionEnded } = renderDialog(vi.fn<Submit>().mockResolvedValue({ sessionRenewed: false }));
    await screen.findByRole("dialog");
    await fill(user, "actual-123", "nueva-larga-1", "nueva-larga-1");
    await user.click(save());
    await vi.waitFor(() => expect(onSessionEnded).toHaveBeenCalled());
  });
});
