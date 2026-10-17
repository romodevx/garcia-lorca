import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { UniformItem } from "../../uniforms/api";
import { CatalogPanel } from "./CatalogPanel";

const CAMPO = { selector: "input, select, textarea" } as const;

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

const book: UniformItem = {
  id: "it-book", name: "LIBRO 3°", description: "", levelId: "", priceCents: 45000,
  active: true, category: "book", sizes: [],
};

type Call = [string, RequestInit | undefined];

function stubWrites(responses: Record<string, Response>): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
    const url = String(input);
    const hit = Object.keys(responses).find((path) => url.endsWith(path));
    return Promise.resolve(hit !== undefined ? responses[hit]!.clone() : jsonResponse({}));
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function posts(fetchMock: ReturnType<typeof vi.fn>, suffix: string): Call[] {
  return (fetchMock.mock.calls as Call[]).filter(([url, init]) => init?.method === "POST" && url.endsWith(suffix));
}

/**
 * ISSUE-CC-112: a book created with "Crear y poner tallas" and the size drawer
 * cancelled had no stock row, so it never showed in Existencias nor in Entrada
 * de almacén — it could be neither stocked nor sold. Something with no sizes
 * is sold as ONE size, so the article gets it at creation.
 */
describe("CatalogPanel — un artículo sin tallas es talla única", () => {
  it("creates a book with its one size, without asking for a size run", async () => {
    const fetchMock = stubWrites({
      "/uniforms/items": jsonResponse(book),
      "/uniforms/items/it-book/sizes": jsonResponse({ createdCount: 1, updatedCount: 0 }),
    });
    const onChanged = vi.fn();
    const user = userEvent.setup();
    render(<CatalogPanel items={[]} levels={[]} onChanged={onChanged} />);

    await user.click(screen.getAllByRole("button", { name: /Nuevo artículo/ })[0]!);
    await user.click(screen.getByRole("button", { name: "Libro" }));
    await user.type(screen.getByLabelText(/Nombre/, CAMPO), "Libro 3°");
    await user.type(screen.getByLabelText(/Precio/, CAMPO), "450");
    await user.click(screen.getByRole("button", { name: "Crear" }));

    await waitFor(() => expect(posts(fetchMock, "/items/it-book/sizes")).toHaveLength(1));
    const body = JSON.parse(posts(fetchMock, "/items/it-book/sizes")[0]![1]!.body as string) as unknown;
    expect(body).toEqual({ sizes: [{ sizeLabel: "ÚNICA", sizeOrder: 1, minStock: 0 }] });
    expect(screen.queryByRole("dialog", { name: /Tallas de/ })).toBeNull();
    expect(await screen.findByText(/talla única/i)).toBeInTheDocument();
    expect(onChanged).toHaveBeenCalled();
  });

  // Gate nit on #142: once the article exists, editing its fields would change
  // nothing; the drawer says what is left and only retries the size.
  it("says the article already exists when only its size failed, and retries just the size", async () => {
    let sizeAttempts = 0;
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/uniforms/items")) return Promise.resolve(jsonResponse(book));
      sizeAttempts += 1;
      return Promise.resolve(sizeAttempts === 1
        ? jsonResponse({ error: { code: "internal", message: "Algo salió mal. Ya lo estamos revisando" } }, 500)
        : jsonResponse({ createdCount: 1, updatedCount: 0 }));
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    render(<CatalogPanel items={[]} levels={[]} onChanged={vi.fn()} />);

    await user.click(screen.getAllByRole("button", { name: /Nuevo artículo/ })[0]!);
    await user.click(screen.getByRole("button", { name: "Libro" }));
    await user.type(screen.getByLabelText(/Nombre/, CAMPO), "Libro 3°");
    await user.type(screen.getByLabelText(/Precio/, CAMPO), "450");
    await user.click(screen.getByRole("button", { name: "Crear" }));

    const drawer = await screen.findByRole("dialog", { name: "Nuevo artículo" });
    expect(await within(drawer).findByText("El artículo ya se creó; falta la talla. Vuelve a intentarlo.")).toBeInTheDocument();
    expect(within(drawer).getByLabelText(/Nombre/, CAMPO)).toBeDisabled();
    await user.click(within(drawer).getByRole("button", { name: "Poner talla única" }));

    await waitFor(() => expect(posts(fetchMock, "/items/it-book/sizes")).toHaveLength(2));
    expect(posts(fetchMock, "/uniforms/items")).toHaveLength(1);
  });

  it("still asks a garment for its size run", async () => {
    stubWrites({ "/uniforms/items": jsonResponse({ ...book, id: "it-polo", name: "POLO", category: "uniform" }) });
    const user = userEvent.setup();
    render(<CatalogPanel items={[]} levels={[]} onChanged={vi.fn()} />);

    await user.click(screen.getAllByRole("button", { name: /Nuevo artículo/ })[0]!);
    await user.type(screen.getByLabelText(/Nombre/, CAMPO), "Polo");
    await user.type(screen.getByLabelText(/Precio/, CAMPO), "320");
    await user.click(screen.getByRole("button", { name: "Crear y poner tallas" }));

    expect(await screen.findByRole("dialog", { name: "Tallas de POLO" })).toBeInTheDocument();
  });

  it("offers one size for an article already left without sizes", async () => {
    const fetchMock = stubWrites({ "/uniforms/items/it-book/sizes": jsonResponse({ createdCount: 1, updatedCount: 0 }) });
    const user = userEvent.setup();
    render(<CatalogPanel items={[book]} levels={[]} onChanged={vi.fn()} />);

    await user.click(screen.getByRole("button", { name: "Tallas" }));
    const drawer = screen.getByRole("dialog", { name: "Tallas de LIBRO 3°" });
    await user.click(within(drawer).getByRole("button", { name: "Usar talla única" }));
    await user.click(within(drawer).getByRole("button", { name: "Guardar 1 talla" }));

    await waitFor(() => expect(posts(fetchMock, "/items/it-book/sizes")).toHaveLength(1));
  });
});

describe("CatalogPanel — un error no cierra el cajón", () => {
  // ISSUE-CC-113: the drawer closed on any failure, losing what was typed.
  it("keeps a new article's drawer open with its fields when creating fails", async () => {
    stubWrites({ "/uniforms/items": jsonResponse({ error: { code: "internal", message: "Algo salió mal. Ya lo estamos revisando" } }, 500) });
    const user = userEvent.setup();
    render(<CatalogPanel items={[]} levels={[]} onChanged={vi.fn()} />);

    await user.click(screen.getAllByRole("button", { name: /Nuevo artículo/ })[0]!);
    await user.type(screen.getByLabelText(/Nombre/, CAMPO), "Polo");
    await user.type(screen.getByLabelText(/Precio/, CAMPO), "320");
    await user.click(screen.getByRole("button", { name: "Crear y poner tallas" }));

    const drawer = await screen.findByRole("dialog", { name: "Nuevo artículo" });
    expect(await within(drawer).findByRole("alert")).toHaveTextContent("Algo salió mal");
    expect(within(drawer).getByLabelText(/Nombre/, CAMPO)).toHaveValue("POLO");
  });
});

// ISSUE-CC-122: "Guardar 1 tallas".
describe("CatalogPanel — el botón cuenta bien", () => {
  it("says 'Guardar 1 talla' in the singular", async () => {
    stubWrites({});
    const user = userEvent.setup();
    render(<CatalogPanel items={[book]} levels={[]} onChanged={vi.fn()} />);

    await user.click(screen.getByRole("button", { name: "Tallas" }));
    await user.type(screen.getByLabelText("Tallas", CAMPO), "M");

    expect(screen.getByRole("button", { name: "Guardar 1 talla" })).toBeInTheDocument();
  });
});
