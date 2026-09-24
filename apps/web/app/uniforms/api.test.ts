import { afterEach, describe, expect, it, vi } from "vitest";
import {
  adjustStock,
  counterSale,
  createItem,
  getSettings,
  listItems,
  listOrders,
  listStock,
  receiveStock,
  updateSettings,
  UniformsError,
  upsertSizes,
} from "./api";

const BASE = "http://gw";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

function lastCall(mock: ReturnType<typeof vi.fn>): [string, RequestInit] {
  const call = mock.mock.calls[0] as [string, RequestInit];
  return [call[0], call[1]];
}

const size = {
  id: "st-1",
  itemId: "it-1",
  sizeLabel: "8",
  sizeOrder: 1,
  onHand: 6,
  reserved: 4,
  available: 2,
  minStock: 3,
  belowMinimum: true,
};

describe("the catalogue", () => {
  it("narrows to what a pupil of a level wears", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ items: [] }));
    vi.stubGlobal("fetch", fetchMock);

    await listItems(BASE, { levelId: "lvl-kinder" });

    const [urlUsed] = lastCall(fetchMock);
    expect(urlUsed).toBe("http://gw/api/v1/uniforms/items?levelId=lvl-kinder");
  });

  it("omits an empty level rather than sending a blank filter", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ items: [] }));
    vi.stubGlobal("fetch", fetchMock);

    await listItems(BASE, { levelId: "" });

    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/uniforms/items");
  });

  it("survives an envelope with no items key rather than throwing", async () => {
    // A defensive default: a null array is the shape that has crashed this
    // codebase's admin surface twice before.
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({}));
    vi.stubGlobal("fetch", fetchMock);

    await expect(listItems(BASE)).resolves.toEqual([]);
  });

  it("sends a new garment and keeps the session cookie", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ id: "it-1" }));
    vi.stubGlobal("fetch", fetchMock);

    await createItem(BASE, { name: "Suéter", priceCents: 65000 });

    const [urlUsed, init] = lastCall(fetchMock);
    expect(urlUsed).toBe("http://gw/api/v1/uniforms/items");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
  });

  it("writes a whole size run in one call", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ createdCount: 3, updatedCount: 0, sizes: [] }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await upsertSizes(BASE, "it-1", [
      { sizeLabel: "4", sizeOrder: 1, minStock: 0 },
      { sizeLabel: "6", sizeOrder: 2, minStock: 0 },
      { sizeLabel: "8", sizeOrder: 3, minStock: 5 },
    ]);

    const [urlUsed, init] = lastCall(fetchMock);
    expect(urlUsed).toBe("http://gw/api/v1/uniforms/items/it-1/sizes");
    expect(JSON.parse(init.body as string).sizes).toHaveLength(3);
    // Created and updated stay separate so the panel can say what happened.
    expect(res.createdCount).toBe(3);
  });

  it("escapes an id with awkward characters into the path", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ createdCount: 0, updatedCount: 0, sizes: [] }));
    vi.stubGlobal("fetch", fetchMock);

    await upsertSizes(BASE, "it/1?x", []);

    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/uniforms/items/it%2F1%3Fx/sizes");
  });
});

describe("the storeroom", () => {
  it("asks only for what is below minimum when told to", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ stock: [size] }));
    vi.stubGlobal("fetch", fetchMock);

    const stock = await listStock(BASE, { onlyBelowMinimum: true });

    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/uniforms/stock?onlyBelowMinimum=true");
    // available and belowMinimum arrive computed; the client never derives them,
    // because two places deriving the same number is two chances to disagree.
    expect(stock[0]?.available).toBe(2);
    expect(stock[0]?.belowMinimum).toBe(true);
  });

  it("sends a goods receipt with its note", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ stock: [], allocatedLineIds: [] }));
    vi.stubGlobal("fetch", fetchMock);

    await receiveStock(BASE, [{ stockId: "st-1", qty: 12 }], "Factura 8871");

    const [, init] = lastCall(fetchMock);
    const body = JSON.parse(init.body as string);
    expect(body.lines).toEqual([{ stockId: "st-1", qty: 12 }]);
    expect(body.note).toBe("Factura 8871");
  });

  it("sends a signed adjustment with its reason", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(size));
    vi.stubGlobal("fetch", fetchMock);

    await adjustStock(BASE, "st-1", -2, "dos piezas dañadas");

    const body = JSON.parse(lastCall(fetchMock)[1].body as string);
    expect(body.qtyDelta).toBe(-2);
    expect(body.note).toBe("dos piezas dañadas");
  });
});

describe("the counter sale", () => {
  it("sends the idempotency key as a header", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ id: "ord-1" }, 201));
    vi.stubGlobal("fetch", fetchMock);

    await counterSale(
      BASE,
      { lines: [{ itemId: "it-1", sizeLabel: "8", qty: 1 }], method: "cash", note: "Tío de Zoe" },
      "till-abc",
    );

    const [, init] = lastCall(fetchMock);
    expect((init.headers as Record<string, string>)["Idempotency-Key"]).toBe("till-abc");
  });

  it("has nowhere to put a price", async () => {
    // The shape IS the enforcement: the amount always comes from the shop's
    // catalogue, so a crafted body cannot buy a jumper for one peso.
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ id: "ord-1" }, 201));
    vi.stubGlobal("fetch", fetchMock);

    await counterSale(BASE, { lines: [{ itemId: "it-1", sizeLabel: "8", qty: 2 }], method: "cash" }, "k");

    const line = JSON.parse(lastCall(fetchMock)[1].body as string).lines[0];
    expect(Object.keys(line).sort()).toEqual(["itemId", "qty", "sizeLabel"]);
  });
});

describe("the order list", () => {
  it("passes every filter, repeating status", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ orders: [], totalCount: 0 }));
    vi.stubGlobal("fetch", fetchMock);

    await listOrders(BASE, {
      levelId: "lvl-primaria",
      q: "Zoe",
      statuses: ["ready_for_pickup", "paid"],
      requiresAttention: true,
      from: "2026-09-01",
      to: "2026-09-30",
      sort: "deadline",
      pageSize: 50,
    });

    const [urlUsed] = lastCall(fetchMock);
    expect(urlUsed).toContain("levelId=lvl-primaria");
    expect(urlUsed).toContain("q=Zoe");
    expect(urlUsed).toContain("status=ready_for_pickup");
    expect(urlUsed).toContain("status=paid");
    expect(urlUsed).toContain("requiresAttention=true");
    expect(urlUsed).toContain("sort=deadline");
    expect(urlUsed).toContain("pageSize=50");
  });

  it("only sends desc when asked for the older-first order", async () => {
    // Newest first is the server's default, and the panel is a work queue.
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ orders: [], totalCount: 0 }));
    vi.stubGlobal("fetch", fetchMock);

    await listOrders(BASE, {});
    expect(lastCall(fetchMock)[0]).not.toContain("desc");

    vi.unstubAllGlobals();
    const second = vi.fn().mockResolvedValue(jsonResponse({ orders: [], totalCount: 0 }));
    vi.stubGlobal("fetch", second);
    await listOrders(BASE, { desc: false });
    expect(lastCall(second)[0]).toContain("desc=false");
  });
});

describe("failures", () => {
  it("parses the envelope so the panel can branch on the code", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
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
    );
    vi.stubGlobal("fetch", fetchMock);

    const err = await counterSale(BASE, { lines: [], method: "cash" }, "k").catch((e: unknown) => e);

    expect(err).toBeInstanceOf(UniformsError);
    const typed = err as UniformsError;
    // The UI branches on the code and never on the message: insufficient_stock
    // and conflict are both 409, and only one is the cashier's fault.
    expect(typed.isOutOfStock).toBe(true);
    expect(typed.message).toContain("ya no está disponible");
    expect(typed.detail).toContain("Suéter verde");
  });

  it("tells a forbidden apart, so the panel can say what is missing", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({ error: { code: "forbidden", message: "Necesitas permiso de cobros para vender en mostrador" } }, 403),
    );
    vi.stubGlobal("fetch", fetchMock);

    const err = (await counterSale(BASE, { lines: [], method: "cash" }, "k").catch((e: unknown) => e)) as UniformsError;

    expect(err.isForbidden).toBe(true);
    expect(err.isOutOfStock).toBe(false);
  });

  it("falls back to text for the gateway's older handlers", async () => {
    // The rest of the edge still answers text/plain. One module adopting the
    // envelope must not fork error handling for everyone.
    const fetchMock = vi.fn().mockResolvedValue(new Response("forbidden", { status: 403 }));
    vi.stubGlobal("fetch", fetchMock);

    const err = (await listItems(BASE).catch((e: unknown) => e)) as UniformsError;

    expect(err).toBeInstanceOf(UniformsError);
    expect(err.message).toBe("forbidden");
  });

  it("falls back to the status when the body is empty", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("", { status: 502 }));
    vi.stubGlobal("fetch", fetchMock);

    const err = (await listItems(BASE).catch((e: unknown) => e)) as UniformsError;

    expect(err.message).toBe("HTTP 502");
  });
});

describe("settings", () => {
  it("round-trips the school's limits", async () => {
    const settings = {
      pickupWindowBusinessDays: 10,
      autoCancelEnabled: true,
      maxExtensions: 2,
      maxOpenOrdersPerStudent: 5,
      maxLinesPerOrder: 20,
      maxUnitsPerLine: 5,
      maxLineExpiries: 2,
    };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(settings));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getSettings(BASE)).resolves.toEqual(settings);

    vi.unstubAllGlobals();
    const put = vi.fn().mockResolvedValue(jsonResponse({ ...settings, pickupWindowBusinessDays: 15 }));
    vi.stubGlobal("fetch", put);
    const saved = await updateSettings(BASE, { ...settings, pickupWindowBusinessDays: 15 });
    expect(saved.pickupWindowBusinessDays).toBe(15);
    expect(lastCall(put)[1].method).toBe("PUT");
  });
});

describe("the base url", () => {
  it("tolerates a trailing slash", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ items: [] }));
    vi.stubGlobal("fetch", fetchMock);

    await listItems("http://gw///");

    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/uniforms/items");
  });
});
