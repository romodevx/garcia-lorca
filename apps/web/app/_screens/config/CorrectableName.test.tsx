import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ConfigError } from "../../config/api";
import { CorrectableName, type CorrectableNameProps } from "./CorrectableName";

function setup(over: Partial<CorrectableNameProps> = {}) {
  const props: CorrectableNameProps = {
    name: "3°",
    noun: "grado",
    onRename: vi.fn().mockResolvedValue(undefined),
    onDelete: vi.fn().mockResolvedValue(undefined),
    deleteDescription: "Se borra el grado 3° de PRIMARIA.",
    children: <span>3°</span>,
    ...over,
  };
  const user = userEvent.setup();
  render(<ul><CorrectableName {...props} /></ul>);
  return { user, props };
}

const conflict = (text: string, code: string): ConfigError => new ConfigError(409, text, code);

describe("CorrectableName — rename in place", () => {
  it("the pencil turns the name into a focused input; Enter saves the trimmed name", async () => {
    const { user, props } = setup();
    await user.click(screen.getByRole("button", { name: "Renombrar grado 3°" }));
    const input = screen.getByRole("textbox", { name: "Nuevo nombre de grado 3°" });
    expect(input).toHaveFocus();
    await user.clear(input);
    await user.type(input, " 4° {Enter}");
    expect(props.onRename).toHaveBeenCalledWith("4°");
    await waitFor(() => expect(screen.queryByRole("textbox")).toBeNull());
  });

  it("Guardar saves too, and a second click while saving is ignored", async () => {
    let resolve: () => void = () => {};
    const onRename = vi.fn(() => new Promise<void>((r) => { resolve = r; }));
    const { user } = setup({ onRename });
    await user.click(screen.getByRole("button", { name: "Renombrar grado 3°" }));
    await user.type(screen.getByRole("textbox"), "B");
    const save = screen.getByRole("button", { name: /Guardar/ });
    await user.click(save);
    await user.click(save);
    expect(save).not.toBeDisabled();
    expect(onRename).toHaveBeenCalledTimes(1);
    resolve();
    await waitFor(() => expect(screen.queryByRole("textbox")).toBeNull());
  });

  it("Esc cancels without saving and gives the name back", async () => {
    const { user, props } = setup();
    await user.click(screen.getByRole("button", { name: "Renombrar grado 3°" }));
    await user.type(screen.getByRole("textbox"), "X{Escape}");
    expect(props.onRename).not.toHaveBeenCalled();
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(screen.getByText("3°")).toBeInTheDocument();
  });

  it("shows the server's 409 sentence inline and keeps what was typed", async () => {
    const onRename = vi.fn().mockRejectedValue(conflict("Ya existe un grado con ese nombre en este nivel.", "GRADE_NAME_TAKEN"));
    const { user } = setup({ onRename });
    await user.click(screen.getByRole("button", { name: "Renombrar grado 3°" }));
    const input = screen.getByRole("textbox");
    await user.clear(input);
    await user.type(input, "2°{Enter}");
    expect(await screen.findByRole("alert")).toHaveTextContent("Ya existe un grado con ese nombre en este nivel.");
    expect(screen.getByRole("textbox")).toHaveValue("2°");
    expect(screen.getByRole("textbox")).toHaveAttribute("aria-invalid", "true");
  });

  it("an empty name is said on save, not sent", async () => {
    const { user, props } = setup();
    await user.click(screen.getByRole("button", { name: "Renombrar grado 3°" }));
    await user.clear(screen.getByRole("textbox"));
    await user.click(screen.getByRole("button", { name: /Guardar/ }));
    expect(props.onRename).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toHaveTextContent("Escribe el nombre del grado.");
  });
});

describe("CorrectableName — delete (irreversible: confirm first)", () => {
  it("asks first, naming what is deleted; Cancelar deletes nothing", async () => {
    const { user, props } = setup();
    await user.click(screen.getByRole("button", { name: "Borrar grado 3°" }));
    const dialog = await screen.findByRole("dialog", { name: "¿Borrar el grado 3°?" });
    expect(dialog).toHaveTextContent("Se borra el grado 3° de PRIMARIA.");
    expect(dialog).toHaveTextContent("No se puede deshacer.");
    await user.click(screen.getByRole("button", { name: "Cancelar" }));
    expect(props.onDelete).not.toHaveBeenCalled();
  });

  it("confirming deletes once", async () => {
    const { user, props } = setup();
    await user.click(screen.getByRole("button", { name: "Borrar grado 3°" }));
    await user.click(await screen.findByRole("button", { name: "Borrar grado" }));
    expect(props.onDelete).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("a 409 shows the server's sentence (what blocks it) next to the name", async () => {
    const onDelete = vi.fn().mockRejectedValue(conflict("No se puede borrar el grado 3°: tiene 2 grupos y 8 materias. Bórralos primero.", "GRADE_LEVEL_IN_USE"));
    const { user } = setup({ onDelete });
    await user.click(screen.getByRole("button", { name: "Borrar grado 3°" }));
    await user.click(await screen.findByRole("button", { name: "Borrar grado" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("No se puede borrar el grado 3°: tiene 2 grupos y 8 materias. Bórralos primero.");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("a failure without the server's words gets a plain sentence, not a code", async () => {
    const onDelete = vi.fn().mockRejectedValue(new Error("network"));
    const { user } = setup({ onDelete });
    await user.click(screen.getByRole("button", { name: "Borrar grado 3°" }));
    await user.click(await screen.findByRole("button", { name: "Borrar grado" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("No se pudo borrar el grado 3°. Intenta de nuevo.");
  });
});

describe("CorrectableName — targets", () => {
  it("pencil and trash are at least 44×44", () => {
    setup();
    for (const name of ["Renombrar grado 3°", "Borrar grado 3°"]) {
      const b = screen.getByRole("button", { name });
      expect(b.style.width).toBe("44px");
      expect(b.style.height).toBe("44px");
    }
  });
});
