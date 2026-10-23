import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";

import { ConfirmDialog } from "./ConfirmDialog";

function Host({ onConfirm, onCancel }: { onConfirm: () => void; onCancel: () => void }): ReactElement {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>Abrir</button>
      {open && (
        <ConfirmDialog
          title="Inscribir alumnos"
          confirmLabel="Inscribir"
          onConfirm={() => { onConfirm(); setOpen(false); }}
          onCancel={() => { onCancel(); setOpen(false); }}
        >
          ¿Inscribir 3 alumnos?
        </ConfirmDialog>
      )}
    </>
  );
}

function open(): { onConfirm: ReturnType<typeof vi.fn>; onCancel: ReturnType<typeof vi.fn>; user: ReturnType<typeof userEvent.setup> } {
  const onConfirm = vi.fn();
  const onCancel = vi.fn();
  const user = userEvent.setup();
  render(<Host onConfirm={onConfirm} onCancel={onCancel} />);
  return { onConfirm, onCancel, user };
}

describe("ConfirmDialog", () => {
  it("is a modal dialog named by its title, with the confirm button focused", async () => {
    const { user } = open();
    await user.click(screen.getByRole("button", { name: "Abrir" }));
    const dialog = await screen.findByRole("dialog", { name: "Inscribir alumnos" });
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(dialog).toHaveTextContent("¿Inscribir 3 alumnos?");
    await waitFor(() => expect(screen.getByRole("button", { name: "Inscribir" })).toHaveFocus());
  });

  it("Enter confirms", async () => {
    const { user, onConfirm, onCancel } = open();
    await user.click(screen.getByRole("button", { name: "Abrir" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Inscribir" })).toHaveFocus());
    await user.keyboard("{Enter}");
    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(onCancel).not.toHaveBeenCalled();
  });

  it("Esc cancels and the focus goes back to what opened it", async () => {
    const { user, onConfirm, onCancel } = open();
    await user.click(screen.getByRole("button", { name: "Abrir" }));
    await screen.findByRole("dialog");
    await user.keyboard("{Escape}");
    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(onConfirm).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Abrir" })).toHaveFocus();
  });

  it("'Cancelar' cancels", async () => {
    const { user, onCancel } = open();
    await user.click(screen.getByRole("button", { name: "Abrir" }));
    await user.click(await screen.findByRole("button", { name: "Cancelar" }));
    expect(onCancel).toHaveBeenCalledTimes(1);
  });
});

describe("ConfirmDialog — destructive", () => {
  it("a confirmation that cuts something wears the danger tone, not the lime check", async () => {
    render(<ConfirmDialog title="Quitar el acceso" confirmLabel="Quitar acceso" danger onConfirm={vi.fn()} onCancel={vi.fn()}>Se corta.</ConfirmDialog>);
    const confirm = await screen.findByRole("button", { name: "Quitar acceso" });
    expect(confirm.style.background).toBe("var(--red)");
  });
});
