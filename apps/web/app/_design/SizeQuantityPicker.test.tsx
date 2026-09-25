import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { SizeQuantityPicker, type PickableSize } from "./SizeQuantityPicker";

const SIZES: PickableSize[] = [
  { label: "4", available: 5 },
  { label: "6", available: 0 },
  { label: "8", available: 2 },
  { label: "10", available: 3 },
];

function pick(onAdd = vi.fn(), inCart: Record<string, number> = {}): ReturnType<typeof vi.fn> {
  render(<SizeQuantityPicker sizes={SIZES} unitPriceCents={45000} inCart={inCart} onAdd={onAdd} />);
  return onAdd;
}

describe("SizeQuantityPicker (tarea 11)", () => {
  it("puts one of a size with a tap, and adjusts it with − and +", async () => {
    const user = userEvent.setup();
    pick();

    await user.click(screen.getByRole("button", { name: /Talla 4/ }));
    expect(screen.getByLabelText("Cantidad talla 4")).toHaveTextContent("1");

    await user.click(screen.getByRole("button", { name: "Una más de talla 4" }));
    await user.click(screen.getByRole("button", { name: "Una más de talla 4" }));
    expect(screen.getByLabelText("Cantidad talla 4")).toHaveTextContent("3");

    await user.click(screen.getByRole("button", { name: "Una menos de talla 4" }));
    expect(screen.getByLabelText("Cantidad talla 4")).toHaveTextContent("2");
  });

  it("never goes past what is free, counting what is already in the cart", async () => {
    const user = userEvent.setup();
    pick(vi.fn(), { "10": 2 });

    await user.click(screen.getByRole("button", { name: /Talla 10/ }));
    const more = screen.getByRole("button", { name: "Una más de talla 10" });
    expect(more).toHaveAttribute("aria-disabled", "true");
    await user.click(more);
    expect(screen.getByLabelText("Cantidad talla 10")).toHaveTextContent("1");
  });

  it("marks a size with nothing on the shelf as sold out and does not take it", async () => {
    const user = userEvent.setup();
    pick();

    const soldOut = screen.getByRole("button", { name: /Talla 6/ });
    expect(soldOut).toHaveAttribute("aria-disabled", "true");
    expect(soldOut).toHaveTextContent("agotada");
    await user.click(soldOut);
    expect(screen.queryByLabelText("Cantidad talla 6")).toBeNull();
  });

  it("adds every size with a quantity in one go, and starts over", async () => {
    const user = userEvent.setup();
    const onAdd = pick();

    await user.click(screen.getByRole("button", { name: /Talla 4/ }));
    await user.click(screen.getByRole("button", { name: /Talla 8/ }));
    await user.click(screen.getByRole("button", { name: "Una más de talla 8" }));
    expect(screen.getByTestId("size-pick-summary")).toHaveTextContent("3 piezas · $1,350.00");

    await user.click(screen.getByRole("button", { name: "Agregar al carrito" }));

    expect(onAdd).toHaveBeenCalledTimes(1);
    expect(onAdd).toHaveBeenCalledWith([
      { sizeLabel: "4", qty: 1 },
      { sizeLabel: "8", qty: 2 },
    ]);
    expect(screen.queryByLabelText("Cantidad talla 4")).toBeNull();
  });

  it("drops a size taken back to zero", async () => {
    const user = userEvent.setup();
    const onAdd = pick();

    await user.click(screen.getByRole("button", { name: /Talla 4/ }));
    await user.click(screen.getByRole("button", { name: "Una menos de talla 4" }));
    expect(screen.queryByLabelText("Cantidad talla 4")).toBeNull();

    // Nothing chosen: the button still answers, and says what is missing.
    await user.click(screen.getByRole("button", { name: "Agregar al carrito" }));
    expect(onAdd).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toHaveTextContent("Elige al menos una talla");
  });

  it("takes a chosen size back out with a second tap", async () => {
    const user = userEvent.setup();
    pick();

    await user.click(screen.getByRole("button", { name: /Talla 4/ }));
    expect(screen.getByRole("button", { name: /Talla 4/ })).toHaveAttribute("aria-pressed", "true");
    await user.click(screen.getByRole("button", { name: /Talla 4/ }));
    expect(screen.queryByLabelText("Cantidad talla 4")).toBeNull();
  });

  it("names the chosen quantity on the chip, as the sighted see it", async () => {
    const user = userEvent.setup();
    pick();

    await user.click(screen.getByRole("button", { name: /Talla 8/ }));
    expect(screen.getByRole("button", { name: /Talla 8/ })).toHaveAccessibleName("Talla 8, 1 elegida de 2 libres");
  });

  it("never sends more than is free when the shelf shrank after choosing", async () => {
    const user = userEvent.setup();
    const onAdd = vi.fn();
    const { rerender } = render(<SizeQuantityPicker sizes={SIZES} unitPriceCents={45000} onAdd={onAdd} />);

    await user.click(screen.getByRole("button", { name: /Talla 4/ }));
    await user.click(screen.getByRole("button", { name: "Una más de talla 4" }));
    await user.click(screen.getByRole("button", { name: "Una más de talla 4" }));
    rerender(<SizeQuantityPicker sizes={[{ label: "4", available: 1 }]} unitPriceCents={45000} onAdd={onAdd} />);
    await user.click(screen.getByRole("button", { name: "Agregar al carrito" }));

    expect(onAdd).toHaveBeenCalledWith([{ sizeLabel: "4", qty: 1 }]);
  });

  it("returns focus to the size's chip when its row goes away", async () => {
    const user = userEvent.setup();
    pick();

    await user.click(screen.getByRole("button", { name: /Talla 4/ }));
    await user.click(screen.getByRole("button", { name: "Una menos de talla 4" }));
    expect(screen.getByRole("button", { name: /Talla 4/ })).toHaveFocus();
  });

  it("sends focus to the first size on sale when nothing was chosen", async () => {
    const user = userEvent.setup();
    pick();

    await user.click(screen.getByRole("button", { name: "Agregar al carrito" }));
    expect(screen.getByRole("button", { name: /Talla 4/ })).toHaveFocus();
  });

  it("says the chosen sizes ran out, rather than that none was chosen", async () => {
    const user = userEvent.setup();
    const onAdd = vi.fn();
    const { rerender } = render(<SizeQuantityPicker sizes={SIZES} unitPriceCents={45000} onAdd={onAdd} />);

    await user.click(screen.getByRole("button", { name: /Talla 4/ }));
    rerender(<SizeQuantityPicker sizes={[{ label: "4", available: 0 }, { label: "8", available: 2 }]} unitPriceCents={45000} onAdd={onAdd} />);
    await user.click(screen.getByRole("button", { name: "Agregar al carrito" }));

    expect(onAdd).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toHaveTextContent("Ya no queda existencia libre de lo que elegiste");
    expect(screen.queryByLabelText("Cantidad talla 4")).toBeNull();
  });
});
