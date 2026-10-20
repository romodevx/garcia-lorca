import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as uniformsApi from "../../uniforms/api";
import { OrdersPanel } from "./OrdersPanel";

vi.mock("../../uniforms/api", async (importOriginal) => {
  const actual = await importOriginal<typeof uniformsApi>();
  return { ...actual, listOrders: vi.fn(), returnCounterSale: vi.fn() };
});
vi.mock("../../_design/students/roomLabels", () => ({ useRoomLabels: () => new Map() }));
const uniforms = vi.mocked(uniformsApi);

const SALE: uniformsApi.UniformOrder = {
  id: "ord-1", folio: "U-2026-000002", studentName: "PRUEBA CALIDAD ALUMNA DOS", source: "counter", label: "closed",
  totalCents: 25000, createdAt: new Date().toISOString(),
  lines: [{ id: "ln-1", itemId: "it-1", itemName: "PLAYERA DEPORTIVA QA", sizeLabel: "8", qty: 1, unitPriceCents: 25000, status: "sold", fulfillmentId: "ful-1" }],
  fulfillments: [{ id: "ful-1", seq: 1, folio: "U-2026-000002", status: "sold_at_counter", amountCents: 25000, deadlineEstimated: false, requiresAttention: false }],
};

afterEach(() => vi.clearAllMocks());

// C2-015: "Devolver" sits beside "Reimprimir"; the panel opens under the row,
// and once returned the list reloads and says so.
describe("OrdersPanel · devolver", () => {
  it("returns a counter sale from its row", async () => {
    const user = userEvent.setup();
    uniforms.listOrders.mockResolvedValueOnce({ orders: [SALE], totalCount: 1, nextPageToken: "" });
    uniforms.listOrders.mockResolvedValueOnce({
      orders: [{ ...SALE, label: "returned", fulfillments: [{ ...SALE.fulfillments[0]!, status: "returned" }] }], totalCount: 1, nextPageToken: "",
    });
    uniforms.returnCounterSale.mockResolvedValue(SALE);
    render(<OrdersPanel narrow={false} levels={[]} />);

    await user.click(await screen.findByRole("button", { name: "Devolver U-2026-000002" }));
    await user.type(screen.getByLabelText(/^Motivo/), "Talla equivocada");
    await user.click(screen.getByRole("button", { name: "Devolver la venta" }));

    await waitFor(() => expect(uniforms.returnCounterSale).toHaveBeenCalled());
    expect(await screen.findByText("Venta U-2026-000002 devuelta.")).toBeInTheDocument();
    expect(await screen.findByText("Devuelta")).toBeInTheDocument();
  });
});
