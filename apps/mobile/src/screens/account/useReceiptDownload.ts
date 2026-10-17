import { useCallback, useState } from "react";
import { receiptPdfPath, type FamilyMovement } from "../../api";
import { usePdfDownload, type PdfDownloadState } from "../grades/usePdfDownload";

const RECEIPT_MESSAGES = {
  network: "No pudimos descargar el comprobante. Revisa tu conexión e inténtalo de nuevo.",
  server: "La escuela no pudo entregar el comprobante en este momento. Inténtalo más tarde.",
  storage: "No pudimos guardar el comprobante en el teléfono. Libera espacio e inténtalo de nuevo.",
  not_found: "Este comprobante ya no está disponible.",
  session: "Tu sesión terminó. Vuelve a iniciar sesión para descargar el comprobante.",
};

/** The download of the receipt the parent tapped last, for that row to show. */
export interface ReceiptStatus {
  depositId: string;
  state: PdfDownloadState;
}

/**
 * Opens a payment's receipt (ISSUE-T14-107) as the PDF the family shares, with
 * the statement's download-then-share and its error wording. A movement
 * without a deposit has no receipt and is ignored. `status` names the tapped
 * deposit, so its own row says how the download went.
 */
export function useReceiptDownload(studentId: string | null): {
  status: ReceiptStatus | null;
  openReceipt: (movement: FamilyMovement) => void;
} {
  const { state, open } = usePdfDownload(null);
  const [tapped, setTapped] = useState<string | null>(null);
  const openReceipt = useCallback(
    (movement: FamilyMovement) => {
      if (studentId === null || !movement.depositId) return;
      setTapped(movement.depositId);
      open({
        path: receiptPdfPath(studentId, movement.depositId),
        kind: "comprobante",
        studentId,
        fallbackName: movement.folio ? `comprobante-${movement.folio}.pdf` : "comprobante.pdf",
        title: "Comprobante de pago",
        messages: RECEIPT_MESSAGES,
      });
    },
    [open, studentId],
  );
  return { status: tapped === null ? null : { depositId: tapped, state }, openReceipt };
}
