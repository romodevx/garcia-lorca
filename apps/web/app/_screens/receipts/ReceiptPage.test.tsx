import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ReceiptPage } from "./ReceiptPage";

const api = vi.hoisted(() => ({ getReceipt: vi.fn(), downloadReceiptPdf: vi.fn() }));
vi.mock("../../receipts/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../receipts/api")>()),
  getReceipt: api.getReceipt,
  downloadReceiptPdf: api.downloadReceiptPdf,
}));

const receipt = {
  kind: "payment", folio: "P-2026-000042", school: { name: "COLEGIO", legalName: "", rfc: "", cct: "", address: "", phone: "", legend: "" },
  paidOn: "2026-10-10", buyer: "Camila", lines: [], surchargeCents: 0, discountCents: 0, creditCents: 0, totalCents: 350000, method: "Efectivo",
};

beforeEach(() => {
  api.getReceipt.mockReset();
  api.downloadReceiptPdf.mockReset();
  vi.spyOn(window, "print").mockImplementation(() => {});
});
afterEach(() => vi.restoreAllMocks());

describe("ReceiptPage (tarea 12)", () => {
  it("prints the ticket once it has it, when opened to print", async () => {
    api.getReceipt.mockResolvedValue(receipt);
    render(<ReceiptPage kind="payment" id="dep-1" autoPrint />);

    expect(await screen.findByText("P-2026-000042")).toBeInTheDocument();
    await waitFor(() => expect(window.print).toHaveBeenCalledTimes(1));
    expect(api.getReceipt).toHaveBeenCalledWith(expect.any(String), "payment", "dep-1");
  });

  it("does not print by itself otherwise, and downloads the PDF on request", async () => {
    api.getReceipt.mockResolvedValue(receipt);
    api.downloadReceiptPdf.mockResolvedValue(undefined);
    const user = userEvent.setup();
    render(<ReceiptPage kind="payment" id="dep-1" />);

    await screen.findByText("P-2026-000042");
    expect(window.print).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Descargar PDF" }));
    expect(api.downloadReceiptPdf).toHaveBeenCalledWith(expect.any(String), "payment", "dep-1");
  });

  it("says so when the receipt cannot be read", async () => {
    api.getReceipt.mockRejectedValue(new Error("Ese comprobante no existe."));
    render(<ReceiptPage kind="sale" id="ord-9" autoPrint />);

    expect(await screen.findByRole("alert")).toHaveTextContent("Ese comprobante no existe.");
    expect(window.print).not.toHaveBeenCalled();
  });

  // Review of task 12: "80mm auto" is not a page size (Chrome drops the whole
  // rule). The page prints with no margin; the roll's width is the driver's.
  it("declares print CSS the browser keeps", async () => {
    api.getReceipt.mockResolvedValue(receipt);
    const { container } = render(<ReceiptPage kind="payment" id="dep-1" />);
    await screen.findByText("P-2026-000042");

    const css = container.querySelector("style")?.textContent ?? "";
    expect(css).toContain("@page { margin: 0; }");
    expect(css).not.toMatch(/size:\s*80mm auto/);
  });
});
