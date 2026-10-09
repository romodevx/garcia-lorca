import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { UndoToast } from "./UndoToast";

/**
 * El toast SÓLO anuncia. El plazo y el compromiso viven en
 * `useUndoableRemoval`, que es donde se prueban.
 */
describe("UndoToast", () => {
  it("deshace sin haber preguntado nada", async () => {
    const user = userEvent.setup();
    const onUndo = vi.fn();
    render(<UndoToast message="Se quitó INDEPENDENCIA" onUndo={onUndo} />);

    await user.click(screen.getByRole("button", { name: "Deshacer" }));

    expect(onUndo).toHaveBeenCalledTimes(1);
  });

  it("se anuncia sin robar el foco de donde está trabajando la persona", () => {
    render(<UndoToast message="Se quitó INDEPENDENCIA" onUndo={vi.fn()} />);

    expect(screen.getByRole("status")).toHaveTextContent("Se quitó INDEPENDENCIA");
  });

  // Painted inside a panel, a hidden setup step would hide its Deshacer while
  // the eight seconds still run and commit. In document.body it stays reachable,
  // and no transformed ancestor can re-anchor its position: fixed.
  it("vive en document.body, fuera del panel que lo pinta", () => {
    const { container } = render(
      <section hidden>
        <UndoToast message="Se quitó INDEPENDENCIA" onUndo={vi.fn()} />
      </section>,
    );

    const toast = screen.getByRole("status");
    expect(toast.parentElement).toBe(document.body);
    expect(container.contains(toast)).toBe(false);
    expect(toast).toBeVisible();
  });
});
