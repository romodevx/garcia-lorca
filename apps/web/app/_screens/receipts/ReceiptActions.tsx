"use client";

import { useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { API_URL } from "../../api-url";
import { downloadReceiptPdf, ticketPath, type ReceiptKind } from "../../receipts/api";

/**
 * "Imprimir ticket" and "Descargar PDF" for a receipt (task 12). The ticket
 * opens on its own 80 mm page, which prints itself; the PDF downloads through
 * the session.
 */
export function ReceiptActions({ kind, id }: { kind: ReceiptKind; id: string }): ReactElement {
  const [failed, setFailed] = useState(false);
  return (
    <>
      <Btn kind="ghost" type="button" onClick={() => window.open(ticketPath(kind, id), "_blank", "noopener")}>
        Imprimir ticket
      </Btn>
      <Btn
        kind="ghost"
        type="button"
        onClick={() => {
          setFailed(false);
          downloadReceiptPdf(API_URL, kind, id).catch(() => setFailed(true));
        }}
      >
        Descargar PDF
      </Btn>
      {failed && (
        <span role="alert" style={{ fontSize: 13.5, color: "var(--bad-fg)" }}>
          No se pudo descargar el PDF.
        </span>
      )}
    </>
  );
}
