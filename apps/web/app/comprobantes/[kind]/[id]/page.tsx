"use client";

import type { ReactElement } from "react";
import { useParams, useSearchParams } from "next/navigation";

import { ReceiptPage } from "../../../_screens/receipts/ReceiptPage";
import type { ReceiptKind } from "../../../receipts/api";

/**
 * /comprobantes/{payment|sale}/{id}[?print=1] — the printable receipt, outside
 * the app's chrome so only the ticket prints (task 12).
 */
export default function ComprobantePage(): ReactElement {
  const params = useParams<{ kind: string; id: string }>();
  const search = useSearchParams();
  const kind: ReceiptKind | undefined = params.kind === "payment" || params.kind === "sale" ? params.kind : undefined;
  if (kind === undefined) {
    return <p role="alert" style={{ padding: 24, textAlign: "center" }}>Ese comprobante no existe.</p>;
  }
  return <ReceiptPage kind={kind} id={params.id} autoPrint={search.get("print") === "1"} />;
}
