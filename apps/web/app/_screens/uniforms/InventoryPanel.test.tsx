import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { UniformItem } from "../../uniforms/api";
import { InventoryPanel } from "./InventoryPanel";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

const jumper: UniformItem = {
  id: "it-1",
  name: "Suéter verde",
  description: "",
  levelId: "",
  priceCents: 65000,
  active: true,
  category: "uniform",
  sizes: [
    { id: "st-m", itemId: "it-1", sizeLabel: "M", sizeOrder: 2, onHand: 6, reserved: 4, available: 2, minStock: 2, belowMinimum: false },
  ],
};

const HOLD_CONFLICT = {
  error: {
    code: "conflict",
    message: "Eso ya existe",
    details: { reason: "that correction would leave fewer pieces than families are already holding" },
  },
};

function renderPanel(items: UniformItem[] = [jumper]): ReturnType<typeof vi.fn> {
  const onChanged = vi.fn();
  render(<InventoryPanel items={items} narrow={false} onChanged={onChanged} />);
  return onChanged;
}

/**
 * ISSUE-CC-113: a correction that would strip a family's hold came back titled
 * "Eso ya existe" with the service's English under it, and the drawer closed —
 * taking the delta and the reason the person had typed with it.
 */
describe("InventoryPanel — a refused correction", () => {
  it("keeps the drawer open with what was typed, and says why in Spanish", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(HOLD_CONFLICT, 409)));
    const user = userEvent.setup();
    renderPanel();

    await user.click(screen.getByRole("button", { name: "Ajustar Suéter verde talla M" }));
    const drawer = screen.getByRole("dialog", { name: /Ajustar Suéter verde/ });
    await user.type(within(drawer).getByLabelText("Diferencia", { selector: "input" }), "-5");
    await user.type(within(drawer).getByLabelText(/Motivo/, { selector: "input" }), "conteo físico");
    await user.click(within(drawer).getByRole("button", { name: "Guardar ajuste" }));

    const still = await screen.findByRole("dialog", { name: /Ajustar Suéter verde/ });
    expect(within(still).getByRole("alert")).toHaveTextContent(
      "Ese ajuste dejaría menos piezas de las que ya están apartadas para familias.",
    );
    expect(screen.queryByText(/ya existe/i)).toBeNull();
    expect(screen.queryByText(/families/i)).toBeNull();
    expect(within(still).getByLabelText("Diferencia", { selector: "input" })).toHaveValue("-5");
    expect(within(still).getByLabelText(/Motivo/, { selector: "input" })).toHaveValue("conteo físico");
  });
});

describe("InventoryPanel — a refused goods receipt", () => {
  it("keeps the drawer open with the quantities typed", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ error: { code: "internal", message: "Algo salió mal. Ya lo estamos revisando" } }, 500)));
    const user = userEvent.setup();
    renderPanel();

    await user.click(screen.getByRole("button", { name: "Entrada de almacén" }));
    const drawer = screen.getByRole("dialog", { name: "Entrada de almacén" });
    await user.type(within(drawer).getByLabelText("Piezas talla M"), "3");
    await user.click(within(drawer).getByRole("button", { name: /Guardar 3 piezas/ }));

    const still = await screen.findByRole("dialog", { name: "Entrada de almacén" });
    expect(within(still).getByRole("alert")).toHaveTextContent("Algo salió mal");
    expect(within(still).getByLabelText("Piezas talla M")).toHaveValue("3");
  });

  // ISSUE-CC-104: zero pieces sent nothing and said nothing.
  it("says what is missing when every quantity is zero, and sends nothing", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    renderPanel();

    await user.click(screen.getByRole("button", { name: "Entrada de almacén" }));
    const drawer = screen.getByRole("dialog", { name: "Entrada de almacén" });
    await user.type(within(drawer).getByLabelText("Piezas talla M"), "0");
    await user.click(within(drawer).getByRole("button", { name: /Guardar 0 piezas/ }));

    expect(within(drawer).getByRole("alert")).toHaveTextContent("Escribe cuántas piezas llegaron de al menos una talla.");
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

// ISSUE-CC-112: an article with no sizes was simply absent from Existencias.
describe("InventoryPanel — an article with no sizes is not invisible", () => {
  it("says which articles are missing and where to fix them", () => {
    const book: UniformItem = { ...jumper, id: "it-book", name: "LIBRO 3°", category: "book", sizes: [] };
    renderPanel([jumper, book]);

    expect(screen.getByText(/LIBRO 3° no aparece aquí porque no tiene tallas/)).toBeInTheDocument();
  });
});

// ISSUE-CC-122: plural agreement on the receipt button.
describe("InventoryPanel — the receipt button counts right", () => {
  it("says 'Guardar 1 pieza'", async () => {
    vi.stubGlobal("fetch", vi.fn());
    const user = userEvent.setup();
    renderPanel();

    await user.click(screen.getByRole("button", { name: "Entrada de almacén" }));
    await user.type(screen.getByLabelText("Piezas talla M"), "1");

    expect(screen.getByRole("button", { name: "Guardar 1 pieza" })).toBeInTheDocument();
  });
});
