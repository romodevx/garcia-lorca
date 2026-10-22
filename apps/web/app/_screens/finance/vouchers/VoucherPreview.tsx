"use client";

import { useEffect, useState, type ReactElement } from "react";
import { Btn } from "../../../_design/Controls";
import { API_URL } from "../../../api-url";
import { getVoucherFile, type PaymentVoucher } from "../../../finance/vouchers";

type Preview = { state: "loading" } | { state: "failed" } | { state: "ready"; url: string };

const frame = { width: "100%", height: 340, borderRadius: "var(--r-md)", border: "1px solid var(--border)", background: "var(--surface-3)" };

/**
 * The family's file, shown in place. It is fetched with the session (there is
 * no public URL) and previewed from a Blob typed as an image or a PDF — never
 * as a page of the app — the object URL released when the drawer closes.
 */
export function VoucherPreview({ voucher }: { voucher: PaymentVoucher }): ReactElement {
  const [preview, setPreview] = useState<Preview>({ state: "loading" });
  const [reads, setReads] = useState(0);

  useEffect(() => {
    let live = true;
    let url: string | undefined;
    setPreview({ state: "loading" });
    getVoucherFile(API_URL, voucher)
      .then((blob) => {
        if (!live) return;
        url = URL.createObjectURL(blob);
        setPreview({ state: "ready", url });
      })
      .catch(() => { if (live) setPreview({ state: "failed" }); });
    return () => {
      live = false;
      if (url !== undefined) URL.revokeObjectURL(url);
    };
  }, [voucher, reads]);

  const title = `Comprobante enviado por ${voucher.uploadedByName}`;
  if (preview.state === "loading") {
    return <p style={{ color: "var(--text-faint)", fontSize: 14, margin: 0 }}>Cargando el comprobante…</p>;
  }
  if (preview.state === "failed") {
    return (
      <div role="alert" style={{ display: "flex", alignItems: "center", gap: 12 }}>
        <p style={{ flex: 1, color: "var(--bad-fg)", fontSize: 14, margin: 0 }}>No pudimos abrir el archivo del comprobante.</p>
        <Btn kind="ghost" onClick={() => setReads((n) => n + 1)}>Reintentar</Btn>
      </div>
    );
  }
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
      {voucher.fileContentType === "application/pdf"
        ? (
          <object data={preview.url} type="application/pdf" title={title} style={frame}>
            <p style={{ fontSize: 14, padding: 12 }}>Este navegador no muestra el PDF aquí: ábrelo en otra pestaña.</p>
          </object>
        )
        : <img src={preview.url} alt={title} style={{ ...frame, height: "auto", maxHeight: 420, objectFit: "contain" }} />}
      <div>
        <Btn kind="ghost" icon="documents" onClick={() => window.open(preview.url, "_blank", "noopener")}>Abrir en otra pestaña</Btn>
      </div>
    </div>
  );
}
