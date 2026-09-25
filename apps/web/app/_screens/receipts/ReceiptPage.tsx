"use client";

import { useEffect, useRef, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { API_URL } from "../../api-url";
import { downloadReceiptPdf, getReceipt, type Receipt, type ReceiptKind } from "../../receipts/api";
import { ReceiptTicket } from "./ReceiptTicket";

// No margin, no toolbar, no padding: the ticket (72 mm, the printable part of
// an 80 mm roll) is what goes to the printer. The page's size is left to the
// printer's driver — "80mm auto" is not a CSS page size, and a browser drops the
// whole rule over it. Kept here, in the page that prints, so no other screen
// inherits it. The demo environment's banner still prints, on purpose: a
// ticket from the demo must say so.
const PRINT_CSS = `
@page { margin: 0; }
@media print {
  body { background: #fff !important; }
  .receipt-toolbar { display: none !important; }
  .receipt-page { padding: 0 !important; min-height: 0 !important; background: #fff !important; }
}
`;

/**
 * The printable receipt (task 12): the 80 mm ticket, with "Imprimir" and
 * "Descargar PDF" above it. Opened with autoPrint (from ?print=1), it opens the
 * print dialog once the receipt has loaded — with Chrome's --kiosk-printing on
 * the counter's PC, it prints straight to the thermal printer.
 */
export function ReceiptPage({ kind, id, autoPrint = false }: { kind: ReceiptKind; id: string; autoPrint?: boolean }): ReactElement {
  const [receipt, setReceipt] = useState<Receipt | undefined>(undefined);
  const [error, setError] = useState<string | undefined>(undefined);
  const printed = useRef(false);

  useEffect(() => {
    getReceipt(API_URL, kind, id)
      .then(setReceipt)
      .catch((err: unknown) => setError(err instanceof Error ? err.message : "No se pudo leer el comprobante."));
  }, [kind, id]);

  useEffect(() => {
    if (autoPrint && receipt !== undefined && !printed.current) {
      printed.current = true;
      window.print();
    }
  }, [autoPrint, receipt]);

  const download = (): void => {
    downloadReceiptPdf(API_URL, kind, id).catch(() => setError("No se pudo descargar el PDF."));
  };

  return (
    <main className="receipt-page" style={{ minHeight: "100vh", background: "var(--surface-2)", padding: "16px" }}>
      <style>{PRINT_CSS}</style>
      <div className="receipt-toolbar" style={{ display: "flex", gap: 10, justifyContent: "center", marginBottom: 16 }}>
        <Btn kind="primary" onClick={() => window.print()}>Imprimir</Btn>
        <Btn kind="outline" onClick={download}>Descargar PDF</Btn>
      </div>
      {error !== undefined && (
        <p role="alert" style={{ textAlign: "center", color: "var(--bad-fg)", fontSize: 15 }}>{error}</p>
      )}
      {receipt !== undefined && <ReceiptTicket receipt={receipt} />}
    </main>
  );
}
