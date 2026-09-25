import { authFetch } from "../auth/authFetch";

/**
 * Receipts (task 12): proof of what a counter deposit or a uniforms sale took,
 * as the gateway assembles it (`/api/v1/receipts/{kind}/{id}`). The JSON feeds
 * the 80 mm ticket the browser prints; the PDF is fetched as bytes through the
 * session (a plain link would not renew an expired one) and saved.
 */

export type ReceiptKind = "payment" | "sale";

export interface ReceiptSchool {
  name: string;
  legalName: string;
  rfc: string;
  cct: string;
  address: string;
  phone: string;
  legend: string;
}

export interface ReceiptLine {
  description: string;
  amountCents: number;
  surcharge?: boolean;
  discountCents?: number;
}

export interface Receipt {
  kind: ReceiptKind;
  folio: string;
  school: ReceiptSchool;
  paidOn: string;
  buyer: string;
  buyerDetail?: string;
  lines: ReceiptLine[];
  surchargeCents: number;
  discountCents: number;
  creditCents: number;
  totalCents: number;
  method: string;
  reference?: string;
  receivedBy?: string;
  voided?: boolean;
  voidReason?: string;
}

/** A receipt the gateway would not give, with its status. */
export class ReceiptError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ReceiptError";
    this.status = status;
  }
}

// Ids are uuids; anything else never reaches the network (it would be a path).
const ID_SHAPE = /^[0-9a-zA-Z-]{1,64}$/;

function receiptURL(baseURL: string, kind: ReceiptKind, id: string, suffix = ""): string {
  if (!ID_SHAPE.test(id)) {
    throw new ReceiptError(400, "Ese comprobante no existe.");
  }
  return `${baseURL.replace(/\/+$/, "")}/api/v1/receipts/${kind}/${encodeURIComponent(id)}${suffix}`;
}

/** GET a receipt as JSON. */
export async function getReceipt(baseURL: string, kind: ReceiptKind, id: string): Promise<Receipt> {
  const res = await authFetch(receiptURL(baseURL, kind, id), { credentials: "include" });
  if (!res.ok) {
    throw new ReceiptError(res.status, res.status === 404 ? "Ese comprobante no existe." : "No se pudo leer el comprobante.");
  }
  return (await res.json()) as Receipt;
}

/** GET a receipt's PDF, with the file name the gateway gave it. */
export async function receiptPdf(baseURL: string, kind: ReceiptKind, id: string): Promise<{ blob: Blob; filename: string }> {
  const res = await authFetch(receiptURL(baseURL, kind, id, "/pdf"), { credentials: "include" });
  if (!res.ok) {
    throw new ReceiptError(res.status, "No se pudo descargar el PDF.");
  }
  const named = /filename="([^"]+)"/.exec(res.headers.get("Content-Disposition") ?? "");
  return { blob: await res.blob(), filename: named?.[1] ?? "comprobante.pdf" };
}

/** Saves a receipt's PDF through the browser's own download. */
export async function downloadReceiptPdf(baseURL: string, kind: ReceiptKind, id: string): Promise<void> {
  const { blob, filename } = await receiptPdf(baseURL, kind, id);
  const href = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = href;
  link.download = filename;
  link.click();
  URL.revokeObjectURL(href);
}

/** Where the printable ticket lives; ?print=1 opens the print dialog. */
export function ticketPath(kind: ReceiptKind, id: string): string {
  return `/comprobantes/${kind}/${encodeURIComponent(id)}?print=1`;
}
