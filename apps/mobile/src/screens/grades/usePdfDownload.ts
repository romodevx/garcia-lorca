import { useCallback, useRef, useState } from "react";
import { downloadPdf, PdfError, sharePdf, type PdfErrorKind } from "../../files/reportCardPdf";

export type PdfDownloadState =
  | { status: "idle" }
  | { status: "downloading"; progress: number | null }
  | { status: "failed"; message: string };

export interface PdfDownloadTarget {
  path: string;
  fallbackName: string;
  title: string;
}

const MESSAGES: Record<PdfErrorKind, string> = {
  network: "No pudimos descargar la boleta. Revisa tu conexión e inténtalo de nuevo.",
  not_found: "Esta boleta ya no está disponible. Puede que la escuela haya emitido una versión nueva.",
  session: "Tu sesión terminó. Vuelve a iniciar sesión para descargar la boleta.",
  unavailable: "Este teléfono no puede abrir el PDF. Inténtalo desde otro dispositivo.",
};

/**
 * Download-then-share for one PDF: progress while it downloads, a plain-words
 * message when it fails. A second tap while downloading is ignored.
 */
export function usePdfDownload(target: PdfDownloadTarget | null): { state: PdfDownloadState; start: () => void } {
  const [state, setState] = useState<PdfDownloadState>({ status: "idle" });
  const busy = useRef(false);

  const start = useCallback(() => {
    if (target === null || busy.current) {
      return;
    }
    busy.current = true;
    setState({ status: "downloading", progress: null });
    void (async () => {
      try {
        const uri = await downloadPdf({
          path: target.path,
          fallbackName: target.fallbackName,
          onProgress: (progress) => setState({ status: "downloading", progress }),
        });
        await sharePdf(uri, target.title);
        setState({ status: "idle" });
      } catch (failure) {
        const kind = failure instanceof PdfError ? failure.kind : "network";
        setState({ status: "failed", message: MESSAGES[kind] });
      } finally {
        busy.current = false;
      }
    })();
  }, [target]);

  return { state, start };
}
