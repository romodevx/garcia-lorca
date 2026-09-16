import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { UndoToast } from "./UndoToast";

beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));
afterEach(() => vi.useRealTimers());

describe("UndoToast", () => {
  it("deshace sin haber preguntado nada", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onUndo = vi.fn();
    const onCommit = vi.fn();
    render(<UndoToast message="Se quitó INDEPENDENCIA" onUndo={onUndo} onCommit={onCommit} />);

    await user.click(screen.getByRole("button", { name: "Deshacer" }));

    expect(onUndo).toHaveBeenCalledTimes(1);
    expect(onCommit).not.toHaveBeenCalled();
  });

  it("confirma el borrado cuando se acaba el plazo", () => {
    const onUndo = vi.fn();
    const onCommit = vi.fn();
    render(<UndoToast message="Se quitó INDEPENDENCIA" onUndo={onUndo} onCommit={onCommit} />);

    act(() => {
      vi.advanceTimersByTime(8000);
    });

    expect(onCommit).toHaveBeenCalledTimes(1);
    expect(onUndo).not.toHaveBeenCalled();
  });

  // Ocho segundos es el plazo, no una sugerencia: a los siete todavía se puede
  // deshacer.
  it("no confirma antes de tiempo", () => {
    const onCommit = vi.fn();
    render(<UndoToast message="Se quitó INDEPENDENCIA" onUndo={vi.fn()} onCommit={onCommit} />);

    act(() => {
      vi.advanceTimersByTime(7000);
    });

    expect(onCommit).not.toHaveBeenCalled();
  });

  it("se anuncia sin robar el foco de donde está trabajando la persona", () => {
    render(<UndoToast message="Se quitó INDEPENDENCIA" onUndo={vi.fn()} onCommit={vi.fn()} />);

    expect(screen.getByRole("status")).toHaveTextContent("Se quitó INDEPENDENCIA");
  });
});
