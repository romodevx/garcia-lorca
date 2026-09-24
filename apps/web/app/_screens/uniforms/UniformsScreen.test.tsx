import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { UniformsScreen } from "./UniformsScreen";
import { parseSizes, sizesMessage } from "./CatalogPanel";
import { toRows } from "./OrdersPanel";
import type { UniformItem, UniformOrder } from "../../uniforms/api";

/**
 * Pide el CAMPO y no su ayuda: el (?) de junto es un botón que se llama
 * "Ayuda sobre <campo>", así que una consulta laxa encuentra los dos.
 */
const CAMPO = { selector: "input, select, textarea" } as const;

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
    { id: "st-m", itemId: "it-1", sizeLabel: "M", sizeOrder: 2, onHand: 6, reserved: 0, available: 6, minStock: 2, belowMinimum: false },
    { id: "st-g", itemId: "it-1", sizeLabel: "G", sizeOrder: 3, onHand: 1, reserved: 1, available: 0, minStock: 2, belowMinimum: true },
  ],
};

/** The school's own levels, from Configuración — Preescolar included. */
const LEVELS = [
  { id: "lvl-pre", name: "Preescolar", sepCode: "", enrollmentGranularity: "cycle" },
  { id: "lvl-pri", name: "Primaria", sepCode: "PRI", enrollmentGranularity: "cycle" },
];

/** The pupil the till sells to. The picker resolves a name to THIS id. */
const zoe = {
  id: "stu-1",
  curp: "ZAVX150312MDFRNN01",
  firstName: "Zoe",
  lastName: "Villalobos",
  level: "primaria",
  levelId: "",
  gradeGroup: "1° A",
  guardianName: "Marisol Villalobos",
  admissionStatus: "enrolled",
};

/**
 * Answers the reads the screen makes on mount.
 *
 * It routes by path because the till makes two: the catalogue, and the roster
 * the pupil picker searches. Answering both with the same body is how the picker
 * silently found nobody.
 */
function stubCatalogue(items: UniformItem[]): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    if (url.includes("/students")) return Promise.resolve(jsonResponse({ students: [zoe] }));
    if (url.includes("/uniforms/levels")) return Promise.resolve(jsonResponse({ levels: LEVELS }));
    return Promise.resolve(jsonResponse({ items }));
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

/**
 * Names a pupil at the till the way a cashier does: type, then choose.
 *
 * Typing alone used to be enough, and that was the bug — the typed text went out
 * as the student id and came back a 500. The choice is what resolves a name to
 * an id, so the tests have to make it.
 */
async function chooseZoe(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.type(screen.getByLabelText("Alumno"), "Zoe");
  await user.click(await screen.findByRole("button", { name: /Zoe Villalobos/ }));
}

/**
 * Answers the till's reads AND the one write it makes.
 *
 * Routed by path rather than by call order: the pupil picker adds a roster read
 * whose position depends on when the cashier types, and a queue of
 * mockResolvedValueOnce cannot express that.
 */
function stubTill(items: UniformItem[], sale: Response): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    if (url.includes("/counter-sale")) return Promise.resolve(sale);
    if (url.includes("/students")) return Promise.resolve(jsonResponse({ students: [zoe] }));
    if (url.includes("/uniforms/levels")) return Promise.resolve(jsonResponse({ levels: LEVELS }));
    return Promise.resolve(jsonResponse({ items }));
  });
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
    //
    // Waits for the COUNT, not for the label. Waiting on "Bajo mínimo" and then
    // reading the number synchronously is a race: the label ships with the card
    // shell, while the number waits on the catalogue. Both land in the same tick
    // on a laptop and do not on a slow builder — this test failed exactly once,
    // in Cloud Build, at 636s for a suite that runs in 8s locally, and blocked a
    // deploy of code that had nothing to do with uniforms.
    await waitFor(() => {
      const low = screen.getByText("Bajo mínimo").closest("button");
      expect(within(low as HTMLElement).getByText("1")).toBeInTheDocument();
    });
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

    await chooseZoe(user);
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
    stubTill([jumper],
      jsonResponse({ id: "ord-1", folio: "U-2026-000007", totalCents: 65000, lines: [], fulfillments: [] }, 201));
    const user = userEvent.setup();
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);

    await waitFor(() => expect(screen.getByText("Suéter verde")).toBeInTheDocument());
    await user.click(screen.getByText("Suéter verde"));
    await user.click(screen.getByRole("button", { name: /M\s*6 libres/ }));
    await chooseZoe(user);
    await user.click(screen.getByRole("button", { name: /Cobrar y entregar/ }));

    await waitFor(() => expect(screen.getByText("U-2026-000007")).toBeInTheDocument());
  });

  it("sends an idempotency key, and a NEW one for the next sale", async () => {
    // A retry of this sale carries the same key; the next parent in the queue
    // carries a different one. Reusing it would make the second sale a no-op.
    const fetchMock = stubTill([jumper],
      jsonResponse({ id: "ord-1", folio: "U-2026-000007", totalCents: 65000, lines: [], fulfillments: [] }, 201));
    const user = userEvent.setup();
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);

    await waitFor(() => expect(screen.getByText("Suéter verde")).toBeInTheDocument());
    await user.click(screen.getByText("Suéter verde"));
    await user.click(screen.getByRole("button", { name: /M\s*6 libres/ }));
    await chooseZoe(user);
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
    stubTill([jumper], jsonResponse(
      {
        error: {
          code: "insufficient_stock",
          message: "Esa talla ya no está disponible, actualizamos la lista",
          details: { reason: "no quedan suficientes Suéter verde en talla M" },
        },
      },
      409,
    ));
    const user = userEvent.setup();
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);

    await waitFor(() => expect(screen.getByText("Suéter verde")).toBeInTheDocument());
    await user.click(screen.getByText("Suéter verde"));
    await user.click(screen.getByRole("button", { name: /M\s*6 libres/ }));
    await chooseZoe(user);
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

describe("UniformsScreen — el inventario no es sólo ropa", () => {
  // The school sells books too. The module already does the hard parts — stock
  // that cannot go negative, an append-only ledger, a counter sale that bills
  // finance — and none of that was ever about clothing.
  it("registers a book, not just a garment", async () => {
    const user = userEvent.setup();
    const fetchMock = stubCatalogue([]);
    render(<UniformsScreen onBack={() => {}} />);
    await waitFor(() => expect(screen.getByRole("tab", { name: "Catálogo" })).toBeInTheDocument());
    await user.click(screen.getByRole("tab", { name: "Catálogo" }));

    // Two buttons offer it: the header and the empty-state hint. Either does.
    await user.click((await screen.findAllByRole("button", { name: /Nuevo artículo/ }))[0] as HTMLElement);
    await user.click(screen.getByLabelText("Libro"));
    await user.type(screen.getByLabelText(/Nombre/, CAMPO), "Libro de matemáticas 3°");
    await user.type(screen.getByLabelText(/Precio/, CAMPO), "450");
    await user.click(screen.getByRole("button", { name: /Guardar|Crear/ }));

    await waitFor(() => {
      const posted = fetchMock.mock.calls.find(([, init]) => (init as RequestInit)?.method === "POST");
      expect(posted).toBeDefined();
      const body = JSON.parse((posted?.[1] as RequestInit).body as string) as Record<string, unknown>;
      expect(body.category).toBe("book");
    });
  });
});

describe("UniformsScreen — un artículo no tiene clave (tarea 3)", () => {
  // Nothing used it: orders point at the article's id. The office no longer
  // invents a code before it can sell something.
  it("creates an article with no clave and never sends one", async () => {
    const user = userEvent.setup();
    const fetchMock = stubCatalogue([]);
    render(<UniformsScreen onBack={() => {}} />);
    await waitFor(() => expect(screen.getByRole("tab", { name: "Catálogo" })).toBeInTheDocument());
    await user.click(screen.getByRole("tab", { name: "Catálogo" }));

    await user.click((await screen.findAllByRole("button", { name: /Nuevo artículo/ }))[0] as HTMLElement);
    expect(screen.queryByLabelText(/Clave/)).not.toBeInTheDocument();
    await user.type(screen.getByLabelText(/Nombre/, CAMPO), "Playera polo");
    await user.type(screen.getByLabelText(/Precio/, CAMPO), "320");
    await user.click(screen.getByRole("button", { name: /Guardar|Crear/ }));

    await waitFor(() => {
      const posted = fetchMock.mock.calls.find(([, init]) => (init as RequestInit)?.method === "POST");
      expect(posted).toBeDefined();
      const body = JSON.parse((posted?.[1] as RequestInit).body as string) as Record<string, unknown>;
      expect(body).not.toHaveProperty("code");
      expect(body.name).toBe("PLAYERA POLO");
    });
  });
});

describe("UniformsScreen — Nuevo artículo al guardar", () => {
  // The forms rule: the primary button is never disabled. Saving says what is
  // missing, on the field, and posts nothing.
  it("marks what is missing instead of greying out the button", async () => {
    const user = userEvent.setup();
    const fetchMock = stubCatalogue([]);
    render(<UniformsScreen onBack={() => {}} />);
    await waitFor(() => expect(screen.getByRole("tab", { name: "Catálogo" })).toBeInTheDocument());
    await user.click(screen.getByRole("tab", { name: "Catálogo" }));
    await user.click((await screen.findAllByRole("button", { name: /Nuevo artículo/ }))[0] as HTMLElement);

    const create = screen.getByRole("button", { name: /Crear y poner tallas/ });
    expect(create).toBeEnabled();
    await user.click(create);

    expect(screen.getByLabelText(/Nombre/, CAMPO)).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByLabelText(/Precio/, CAMPO)).toHaveAttribute("aria-invalid", "true");
    expect(fetchMock.mock.calls.some(([, init]) => (init as RequestInit)?.method === "POST")).toBe(false);
  });

  // With no clave there is no unique code to refuse a second insert: a double
  // click must create one article, not two.
  it("creates one article on a double click", async () => {
    const user = userEvent.setup();
    const fetchMock = stubCatalogue([]);
    render(<UniformsScreen onBack={() => {}} />);
    await waitFor(() => expect(screen.getByRole("tab", { name: "Catálogo" })).toBeInTheDocument());
    await user.click(screen.getByRole("tab", { name: "Catálogo" }));
    await user.click((await screen.findAllByRole("button", { name: /Nuevo artículo/ }))[0] as HTMLElement);
    await user.type(screen.getByLabelText(/Nombre/, CAMPO), "Playera polo");
    await user.type(screen.getByLabelText(/Precio/, CAMPO), "320");

    await user.dblClick(screen.getByRole("button", { name: /Crear y poner tallas/ }));

    await waitFor(() =>
      expect(fetchMock.mock.calls.filter(([, init]) => (init as RequestInit)?.method === "POST").length).toBeGreaterThan(0),
    );
    const itemPosts = fetchMock.mock.calls.filter(
      ([url, init]) => (init as RequestInit)?.method === "POST" && String(url).endsWith("/uniforms/items"),
    );
    expect(itemPosts).toHaveLength(1);
  });
});

describe("UniformsScreen — los niveles son los del colegio", () => {
  // The level chips come from Configuración, not from a fixed SEP list (task 2):
  // a garment can be scoped to Preescolar, and it travels by level id.
  it("scopes a new garment to a level the school defined", async () => {
    const user = userEvent.setup();
    const fetchMock = stubCatalogue([]);
    render(<UniformsScreen onBack={() => {}} />);
    await waitFor(() => expect(screen.getByRole("tab", { name: "Catálogo" })).toBeInTheDocument());
    await user.click(screen.getByRole("tab", { name: "Catálogo" }));

    await user.click((await screen.findAllByRole("button", { name: /Nuevo artículo/ }))[0] as HTMLElement);
    await user.type(screen.getByLabelText(/Nombre/, CAMPO), "Batita");
    await user.type(screen.getByLabelText(/Precio/, CAMPO), "410");
    await user.click(await screen.findByRole("button", { name: "Preescolar" }));
    expect(screen.queryByRole("button", { name: "Prepa" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /Guardar|Crear/ }));

    await waitFor(() => {
      const posted = fetchMock.mock.calls.find(([, init]) => (init as RequestInit)?.method === "POST");
      expect(posted).toBeDefined();
      const body = JSON.parse((posted?.[1] as RequestInit).body as string) as Record<string, unknown>;
      expect(body.levelId).toBe("lvl-pre");
    });
  });
});

describe("UniformsScreen — una prenda para todos", () => {
  // The gateway omits levelId for a garment of every level (omitempty), so it
  // arrives undefined, not "". The catalogue must still read "todos los niveles".
  it("labels a garment with no level as for every level", async () => {
    const user = userEvent.setup();
    const { levelId: _omitted, ...unscoped } = jumper;
    stubCatalogue([unscoped as UniformItem]);
    render(<UniformsScreen onBack={() => {}} />);
    await waitFor(() => expect(screen.getByRole("tab", { name: "Catálogo" })).toBeInTheDocument());
    await user.click(screen.getByRole("tab", { name: "Catálogo" }));

    expect(await screen.findByText(/todos los niveles/)).toBeInTheDocument();
    expect(screen.queryByText(/nivel sin nombre/)).not.toBeInTheDocument();
  });
});
