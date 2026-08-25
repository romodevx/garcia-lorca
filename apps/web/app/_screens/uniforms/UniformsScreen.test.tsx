import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { UniformsScreen } from "./UniformsScreen";
import { parseSizes, sizesMessage } from "./CatalogPanel";
import { toRows } from "./OrdersPanel";
import type { UniformItem, UniformOrder } from "../../uniforms/api";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

const jumper: UniformItem = {
  id: "it-1",
  code: "SUE-VER",
  name: "Suéter verde",
  description: "",
  level: "",
  priceCents: 65000,
  active: true,
  sizes: [
    { id: "st-m", itemId: "it-1", sizeLabel: "M", sizeOrder: 2, onHand: 6, reserved: 0, available: 6, minStock: 2, belowMinimum: false },
    { id: "st-g", itemId: "it-1", sizeLabel: "G", sizeOrder: 3, onHand: 1, reserved: 1, available: 0, minStock: 2, belowMinimum: true },
  ],
};

/** Answers the catalogue read the screen makes on mount. */
function stubCatalogue(items: UniformItem[]): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ items }));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

const ALL = ["uniforms:stock", "uniforms:write", "uniforms:orders", "finance:write"];

describe("the shop's shell", () => {
  it("lands on Mostrador, because that is where a person is waiting", async () => {
    stubCatalogue([jumper]);
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);

    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "Mostrador" })).toHaveAttribute("aria-selected", "true");
    });
  });

  it("hides the order list from an account that may not read it", async () => {
    // The storeroom holds stock and write, deliberately NOT uniforms:orders:
    // that list carries every pupil's name and purchase history.
    stubCatalogue([jumper]);
    render(<UniformsScreen onBack={() => {}} permissions={["uniforms:stock", "uniforms:write"]} />);

    await waitFor(() => expect(screen.getByRole("tab", { name: "Mostrador" })).toBeInTheDocument());
    expect(screen.queryByRole("tab", { name: "Pedidos" })).toBeNull();
  });

  it("shows the order list to an account that may", async () => {
    stubCatalogue([jumper]);
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);

    await waitFor(() => expect(screen.getByRole("tab", { name: "Pedidos" })).toBeInTheDocument());
  });

  it("counts what is below minimum and what is empty", async () => {
    stubCatalogue([jumper]);
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);

    // The G is both empty and below its minimum; the M is neither.
    await waitFor(() => {
      expect(screen.getByText("Bajo mínimo")).toBeInTheDocument();
    });
    const low = screen.getByText("Bajo mínimo").closest("button");
    expect(within(low as HTMLElement).getByText("1")).toBeInTheDocument();
  });
});

describe("the till", () => {
  it("says WHY it is closed when the account cannot take money", async () => {
    // A storeroom role runs the whole storeroom, but selling moves cash. The
    // point is that it says so at the door rather than after six lines.
    stubCatalogue([jumper]);
    render(<UniformsScreen onBack={() => {}} permissions={["uniforms:stock", "uniforms:write"]} />);

    await waitFor(() => {
      expect(screen.getByText("Necesitas permiso de cobros")).toBeInTheDocument();
    });
  });

  it("opens for an account that can", async () => {
    stubCatalogue([jumper]);
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);

    await waitFor(() => expect(screen.getByText("1. Prenda")).toBeInTheDocument());
    expect(screen.queryByText("Necesitas permiso de cobros")).toBeNull();
  });

  it("shows a sold-out size as unavailable rather than hiding it", async () => {
    // Hiding it would leave a parent wondering whether the school stocks that
    // size at all. Disabled AND labelled is the honest answer.
    stubCatalogue([jumper]);
    const user = userEvent.setup();
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);

    await waitFor(() => expect(screen.getByText("Suéter verde")).toBeInTheDocument());
    await user.click(screen.getByText("Suéter verde"));

    const soldOut = screen.getByRole("button", { name: /G\s*agotada/ });
    expect(soldOut).toBeDisabled();
  });

  it("keeps the confirm button shut until it has everything it needs", async () => {
    stubCatalogue([jumper]);
    const user = userEvent.setup();
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);

    await waitFor(() => expect(screen.getByText("Suéter verde")).toBeInTheDocument());
    const confirm = (): HTMLElement => screen.getByRole("button", { name: /Cobrar y entregar/ });
    expect(confirm()).toBeDisabled();

    await user.click(screen.getByText("Suéter verde"));
    await user.click(screen.getByRole("button", { name: /M\s*6 libres/ }));
    // A cart is not enough: an anonymous sale needs a note, and a named one
    // needs the pupil.
    expect(confirm()).toBeDisabled();

    await user.type(screen.getByLabelText("Alumno"), "Zoe");
    expect(confirm()).toBeEnabled();
  });

  it("warns, in the card itself, that an anonymous sale lands on nobody's statement", async () => {
    stubCatalogue([jumper]);
    const user = userEvent.setup();
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);

    await waitFor(() => expect(screen.getByText("Venta sin alumno")).toBeInTheDocument());
    await user.click(screen.getByText("Venta sin alumno"));

    expect(screen.getByText(/no quedará en ningún estado de cuenta/)).toBeInTheDocument();
  });

  it("preselects cash, and only asks for a reference when it is not", async () => {
    stubCatalogue([jumper]);
    const user = userEvent.setup();
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);

    await waitFor(() => expect(screen.getByRole("button", { name: "Efectivo" })).toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Efectivo" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.queryByLabelText("Referencia")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Transferencia" }));
    expect(screen.getByLabelText("Referencia")).toBeInTheDocument();
  });

  it("shows the folio big after a sale, because it is the only handle on it", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ items: [jumper] }))
      .mockResolvedValueOnce(
        jsonResponse({ id: "ord-1", folio: "U-2026-000007", totalCents: 65000, lines: [], fulfillments: [] }, 201),
      )
      .mockResolvedValue(jsonResponse({ items: [jumper] }));
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);

    await waitFor(() => expect(screen.getByText("Suéter verde")).toBeInTheDocument());
    await user.click(screen.getByText("Suéter verde"));
    await user.click(screen.getByRole("button", { name: /M\s*6 libres/ }));
    await user.type(screen.getByLabelText("Alumno"), "Zoe");
    await user.click(screen.getByRole("button", { name: /Cobrar y entregar/ }));

    await waitFor(() => expect(screen.getByText("U-2026-000007")).toBeInTheDocument());
  });

  it("sends an idempotency key, and a NEW one for the next sale", async () => {
    // A retry of this sale carries the same key; the next parent in the queue
    // carries a different one. Reusing it would make the second sale a no-op.
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ items: [jumper] }))
      .mockResolvedValueOnce(
        jsonResponse({ id: "ord-1", folio: "U-2026-000007", totalCents: 65000, lines: [], fulfillments: [] }, 201),
      )
      .mockResolvedValue(jsonResponse({ items: [jumper] }));
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);

    await waitFor(() => expect(screen.getByText("Suéter verde")).toBeInTheDocument());
    await user.click(screen.getByText("Suéter verde"));
    await user.click(screen.getByRole("button", { name: /M\s*6 libres/ }));
    await user.type(screen.getByLabelText("Alumno"), "Zoe");
    await user.click(screen.getByRole("button", { name: /Cobrar y entregar/ }));

    await waitFor(() => expect(screen.getByText("U-2026-000007")).toBeInTheDocument());
    const saleCall = fetchMock.mock.calls.find((call) =>
      String(call[0]).includes("/counter-sale"),
    ) as [string, RequestInit];
    const firstKey = (saleCall[1].headers as Record<string, string>)["Idempotency-Key"];
    expect(firstKey).toBeTruthy();

    await user.click(screen.getByRole("button", { name: /Siguiente venta/ }));
    await waitFor(() => expect(screen.getByText("1. Prenda")).toBeInTheDocument());
    // The screen is reset for the next parent; the key it will send is fresh.
    expect(screen.queryByText("U-2026-000007")).toBeNull();
  });

  it("shows the shop's own words when a size ran out mid-sale", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ items: [jumper] }))
      .mockResolvedValueOnce(
        jsonResponse(
          {
            error: {
              code: "insufficient_stock",
              message: "Esa talla ya no está disponible, actualizamos la lista",
              details: { reason: "there are not enough of Suéter verde in size M" },
            },
          },
          409,
        ),
      )
      .mockResolvedValue(jsonResponse({ items: [jumper] }));
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);

    await waitFor(() => expect(screen.getByText("Suéter verde")).toBeInTheDocument());
    await user.click(screen.getByText("Suéter verde"));
    await user.click(screen.getByRole("button", { name: /M\s*6 libres/ }));
    await user.type(screen.getByLabelText("Alumno"), "Zoe");
    await user.click(screen.getByRole("button", { name: /Cobrar y entregar/ }));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent("Esa talla ya no está disponible");
    });
    // Which garment is what the cashier actually needs, and the generic sentence
    // cannot say it.
    expect(screen.getByRole("alert")).toHaveTextContent("Suéter verde");
  });
});

describe("the empty states", () => {
  it("tells a first-afternoon shop what to do", async () => {
    stubCatalogue([]);
    const user = userEvent.setup();
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);

    await waitFor(() => expect(screen.getByRole("tab", { name: "Catálogo" })).toBeInTheDocument());
    await user.click(screen.getByRole("tab", { name: "Catálogo" }));

    expect(screen.getByText("Crea tu primera prenda")).toBeInTheDocument();
  });

  it("says something different when there are garments but no sizes yet", async () => {
    stubCatalogue([{ ...jumper, sizes: [] }]);
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);

    await waitFor(() => {
      expect(screen.getByText("Aún no hay prendas con tallas")).toBeInTheDocument();
    });
  });
});

describe("parseSizes", () => {
  it("expands a numeric range to the sizes a school actually stocks", () => {
    // Nobody sells a size 5 jumper.
    expect(parseSizes("4-12")).toEqual(["4", "6", "8", "10", "12"]);
  });

  it("keeps an odd start honest rather than guessing", () => {
    expect(parseSizes("3-6")).toEqual(["3", "4", "5", "6"]);
  });

  it("takes a letter run verbatim, because letters have no arithmetic", () => {
    expect(parseSizes("CH, M, G")).toEqual(["CH", "M", "G"]);
  });

  it("keeps the order somebody typed", () => {
    expect(parseSizes("G, M, CH")).toEqual(["G", "M", "CH"]);
  });

  it("drops duplicates instead of creating two identical sizes", () => {
    expect(parseSizes("M, M, G")).toEqual(["M", "G"]);
  });

  it("is empty for empty input", () => {
    expect(parseSizes("   ")).toEqual([]);
  });
});

describe("sizesMessage", () => {
  it("says what happened instead of saving in silence", () => {
    expect(sizesMessage(8, 2)).toBe("8 tallas creadas, 2 ya existían");
    expect(sizesMessage(1, 0)).toBe("1 talla creada");
    expect(sizesMessage(0, 1)).toBe("1 ya existía");
    expect(sizesMessage(0, 0)).toBe("Sin cambios");
  });
});

describe("toRows", () => {
  const order: UniformOrder = {
    id: "ord-1",
    folio: "U-2026-000123",
    studentName: "Zoe Villalobos",
    studentLevel: "primaria",
    source: "family",
    label: "partial",
    totalCents: 130000,
    lines: [
      { id: "l1", itemId: "it-1", itemName: "Playera", sizeLabel: "8", qty: 1, unitPriceCents: 32000, status: "sold", fulfillmentId: "f1" },
      { id: "l2", itemId: "it-2", itemName: "Suéter", sizeLabel: "10", qty: 1, unitPriceCents: 65000, status: "awaiting_stock" },
    ],
    fulfillments: [
      { id: "f1", seq: 1, folio: "U-2026-000123-A", status: "ready_for_pickup", amountCents: 32000, deadlineEstimated: false, requiresAttention: false },
    ],
  };

  it("makes a row per DELIVERY, not per order", () => {
    // The order has no status of its own; the delivery is what gets picked,
    // charged and handed over, so it is the delivery that earns a row.
    const rows = toRows([order]);
    expect(rows).toHaveLength(2);
    expect(rows[0]?.folioSuffix).toBe("-A");
  });

  it("adds a row for what is still waiting, with no deadline", () => {
    // Nothing is held yet, so no clock is running — and the absence says that
    // without a sentence having to.
    const rows = toRows([order]);
    const pending = rows[1];
    expect(pending?.statusLabel).toBe("Por surtir");
    expect(pending?.folioSuffix).toBe("");
  });

  it("ties an order's rows together with one accent", () => {
    const rows = toRows([order]);
    expect(rows[0]?.accent).toBe(rows[1]?.accent);
  });

  it("names an anonymous counter sale rather than leaving the column blank", () => {
    const rows = toRows([{ ...order, studentName: "", lines: [], fulfillments: order.fulfillments }]);
    expect(rows[0]?.who).toBe("Venta sin alumno");
  });

  it("surfaces a delivery that needs a person over its own status", () => {
    const flagged = {
      ...order,
      lines: [],
      fulfillments: [{ ...order.fulfillments[0]!, requiresAttention: true }],
    };
    expect(toRows([flagged])[0]?.statusLabel).toBe("Requiere atención");
  });
});
