import { useCallback, useEffect, useRef, useState } from "react";
import { downloadPdf, PdfError, sharePdf, type PdfErrorKind } from "../../files/reportCardPdf";

export type PdfDownloadState =
  | { status: "idle" }
  | { status: "downloading"; progress: number | null }
  | { status: "failed"; message: string };

export interface PdfDownloadTarget {
  path: string;
  studentId: string;
  /** Used when the server's filename is unavailable, from fallbackPdfName. */
  fallbackName: string;
  title: string;
  /** What to say on failure, when the document is not a boleta. */
  messages?: Partial<Record<PdfErrorKind, string>>;
}

const MESSAGES: Record<PdfErrorKind, string> = {
  network: "No pudimos descargar la boleta. Revisa tu conexión e inténtalo de nuevo.",
  server: "La escuela no pudo entregar la boleta en este momento. Inténtalo más tarde.",
  storage: "No pudimos guardar la boleta en el teléfono. Libera espacio e inténtalo de nuevo.",
  not_found: "Esta boleta ya no está disponible. Puede que la escuela haya emitido una versión nueva.",
  session: "Tu sesión terminó. Vuelve a iniciar sesión para descargar la boleta.",
  unavailable: "Este teléfono no puede abrir el PDF. Inténtalo desde otro dispositivo.",
};

/**
 * Download-then-share for one PDF: progress while it downloads, a plain-words
 * message when it fails. A second tap while downloading is ignored. When the
 * screen closes mid-download the result is dropped: no share sheet pops up
 * over whatever the parent went to, and no state is set on an unmounted screen.
 */
export function usePdfDownload(target: PdfDownloadTarget | null): { state: PdfDownloadState; start: () => void } {
  const [state, setState] = useState<PdfDownloadState>({ status: "idle" });
  const busy = useRef(false);
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const start = useCallback(() => {
    if (target === null || busy.current) {
      return;
    }
    busy.current = true;
    setState({ status: "downloading", progress: null });
    const update = (next: PdfDownloadState): void => {
      if (mounted.current) setState(next);
    };
    void (async () => {
      try {
        const uri = await downloadPdf({
          path: target.path,
          studentId: target.studentId,
          fallbackName: target.fallbackName,
          onProgress: (progress) => update({ status: "downloading", progress }),
        });
        if (!mounted.current) return;
        await sharePdf(uri, target.title);
        update({ status: "idle" });
      } catch (failure) {
        const kind = failure instanceof PdfError ? failure.kind : "network";
        update({ status: "failed", message: target.messages?.[kind] ?? MESSAGES[kind] });
      } finally {
        busy.current = false;
      }
    })();
  }, [target]);

  return { state, start };
}
