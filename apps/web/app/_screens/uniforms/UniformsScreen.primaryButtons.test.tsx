import { afterEach, describe, expect, it, vi } from "vitest";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { UniformsScreen } from "./UniformsScreen";
import type { UniformItem } from "../../uniforms/api";

/**
 * QA 2026-10-15, ISSUE-015: "Guardar N tallas", "Guardar N piezas" and
 * "Cobrar y entregar" sat greyed out with no reason. The forms rule: the
 * primary button is never disabled — pressing it says what is missing, and
 * while it saves it is busy and a second press does nothing.
 */
afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const ALL = ["uniforms:stock", "uniforms:write", "uniforms:orders", "finance:write"];

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
  ],
};
const bare: UniformItem = { ...jumper, id: "it-2", name: "Playera polo", sizes: [] };

/** Answers every read; a write waits on `hold` when given, so "busy" can be seen. */
function stub(items: UniformItem[], hold?: Promise<Response>): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    if (init?.method === "POST" || init?.method === "PUT") return hold ?? Promise.resolve(jsonResponse({}));
    if (url.includes("/students")) return Promise.resolve(jsonResponse({ students: [] }));
    if (url.includes("/uniforms/levels")) return Promise.resolve(jsonResponse({ levels: [] }));
    return Promise.resolve(jsonResponse({ items }));
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

const writes = (fetchMock: ReturnType<typeof vi.fn>): number =>
  fetchMock.mock.calls.filter(([, init]) => ["POST", "PUT"].includes((init as RequestInit | undefined)?.method ?? "")).length;

describe("Guardar N tallas", () => {
  it("is never disabled: with no sizes typed it says so and sends nothing", async () => {
    const user = userEvent.setup();
    const fetchMock = stub([bare]);
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);
    await user.click(await screen.findByRole("tab", { name: "Catálogo" }));
    await user.click(await screen.findByRole("button", { name: /Tallas/ }));

    const save = screen.getByRole("button", { name: /Guardar 0 tallas/ });
    expect(save).toBeEnabled();
    await user.click(save);

    expect(screen.getByLabelText("Tallas", { selector: "input" })).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByText(/Escribe las tallas/)).toBeInTheDocument();
    expect(writes(fetchMock)).toBe(0);
  });

  it("is busy while saving and saves once on a double click", async () => {
    const user = userEvent.setup();
    let release!: (r: Response) => void;
    const fetchMock = stub([bare], new Promise<Response>((resolve) => { release = resolve; }));
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);
    await user.click(await screen.findByRole("tab", { name: "Catálogo" }));
    await user.click(await screen.findByRole("button", { name: /Tallas/ }));
    await user.type(screen.getByLabelText("Tallas", { selector: "input" }), "CH, M");

    await user.dblClick(screen.getByRole("button", { name: /Guardar 2 tallas/ }));

    const busy = screen.getByRole("button", { name: /Guardando/ });
    expect(busy).toBeEnabled();
    expect(busy).toHaveAttribute("aria-busy", "true");
    expect(writes(fetchMock)).toBe(1);
    await act(async () => release(jsonResponse({ createdCount: 2, updatedCount: 0 })));
  });
});

describe("Guardar N piezas", () => {
  it("is never disabled: with no quantity typed it says so and sends nothing", async () => {
    const user = userEvent.setup();
    const fetchMock = stub([jumper]);
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);
    await user.click(await screen.findByRole("tab", { name: "Existencias" }));
    await user.click(await screen.findByRole("button", { name: /Entrada de almacén/ }));

    const save = screen.getByRole("button", { name: /Guardar 0 piezas/ });
    expect(save).toBeEnabled();
    await user.click(save);

    expect(screen.getByText(/Escribe cuántas piezas llegaron/)).toBeInTheDocument();
    await waitFor(() => expect(screen.getByLabelText("Piezas talla M")).toHaveFocus());
    expect(writes(fetchMock)).toBe(0);
  });

  it("is busy while saving and books the box in once on a double click", async () => {
    const user = userEvent.setup();
    let release!: (r: Response) => void;
    const fetchMock = stub([jumper], new Promise<Response>((resolve) => { release = resolve; }));
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);
    await user.click(await screen.findByRole("tab", { name: "Existencias" }));
    await user.click(await screen.findByRole("button", { name: /Entrada de almacén/ }));
    await user.type(screen.getByLabelText("Piezas talla M"), "4");

    await user.dblClick(screen.getByRole("button", { name: /Guardar 4 piezas/ }));

    expect(screen.getByRole("button", { name: /Guardando/ })).toHaveAttribute("aria-busy", "true");
    expect(writes(fetchMock)).toBe(1);
    await act(async () => release(jsonResponse({})));
  });
});

describe("Cobrar y entregar", () => {
  it("is never disabled: an empty cart says what is missing and sells nothing", async () => {
    const user = userEvent.setup();
    const fetchMock = stub([jumper]);
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);
    await waitFor(() => expect(screen.getByText("Suéter verde")).toBeInTheDocument());

    const sell = screen.getByRole("button", { name: /Cobrar y entregar/ });
    expect(sell).toBeEnabled();
    await user.click(sell);

    expect(screen.getByText("Agrega al menos una prenda al carrito.")).toBeInTheDocument();
    expect(writes(fetchMock)).toBe(0);
  });

  it("asks for the pupil once the cart is filled", async () => {
    const user = userEvent.setup();
    const fetchMock = stub([jumper]);
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);
    await waitFor(() => expect(screen.getByText("Suéter verde")).toBeInTheDocument());
    await user.click(screen.getByText("Suéter verde"));
    await user.click(screen.getByRole("button", { name: /Talla M, 6 libres/ }));
    await user.click(screen.getByRole("button", { name: "Agregar al carrito" }));

    await user.click(screen.getByRole("button", { name: /Cobrar y entregar/ }));

    expect(screen.getByText(/Elige al alumno/)).toBeInTheDocument();
    await waitFor(() => expect(screen.getByLabelText("Alumno")).toHaveFocus());
    expect(writes(fetchMock)).toBe(0);
  });

  it("an anonymous sale without a buyer asks for one, on the field", async () => {
    const user = userEvent.setup();
    const fetchMock = stub([jumper]);
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);
    await waitFor(() => expect(screen.getByText("Suéter verde")).toBeInTheDocument());
    await user.click(screen.getByText("Suéter verde"));
    await user.click(screen.getByRole("button", { name: /Talla M, 6 libres/ }));
    await user.click(screen.getByRole("button", { name: "Agregar al carrito" }));
    await user.click(screen.getByRole("button", { name: "Venta sin alumno" }));

    await user.click(screen.getByRole("button", { name: /Cobrar y entregar/ }));

    const buyer = screen.getByLabelText(/Quién compró/, { selector: "input" });
    expect(buyer).toHaveAttribute("aria-invalid", "true");
    await waitFor(() => expect(buyer).toHaveFocus());
    expect(writes(fetchMock)).toBe(0);
  });

  it("is busy while charging and sells once on a double click", async () => {
    const user = userEvent.setup();
    let release!: (r: Response) => void;
    const fetchMock = stub([jumper], new Promise<Response>((resolve) => { release = resolve; }));
    render(<UniformsScreen onBack={() => {}} permissions={ALL} />);
    await waitFor(() => expect(screen.getByText("Suéter verde")).toBeInTheDocument());
    await user.click(screen.getByText("Suéter verde"));
    await user.click(screen.getByRole("button", { name: /Talla M, 6 libres/ }));
    await user.click(screen.getByRole("button", { name: "Agregar al carrito" }));
    await user.click(screen.getByRole("button", { name: "Venta sin alumno" }));
    await user.type(screen.getByLabelText(/Quién compró/, { selector: "input" }), "Tía de Zoe");

    await user.dblClick(screen.getByRole("button", { name: /Cobrar y entregar/ }));

    expect(screen.getByRole("button", { name: /Cobrando/ })).toHaveAttribute("aria-busy", "true");
    expect(writes(fetchMock)).toBe(1);
    await act(async () => release(jsonResponse({ id: "ord-1", folio: "U-1", totalCents: 65000, lines: [], fulfillments: [] }, 201)));
  });
});
