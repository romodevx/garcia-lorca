"use client";

import { useRef, useState, type DragEvent, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { Icon } from "../../_design/Icon";
import { alertStyle } from "./parts";

export interface UploadStepProps {
  /** The file being read, while it is. */
  reading: string | undefined;
  error: string | undefined;
  onFiles: (files: readonly File[]) => void;
}

/**
 * Step 1: one .xlsx at a time. The drop zone is a convenience; the button is
 * the keyboard's way in (and the screen reader's), so nothing needs a mouse.
 */
export function UploadStep({ reading, error, onFiles }: UploadStepProps): ReactElement {
  const input = useRef<HTMLInputElement | null>(null);
  const [over, setOver] = useState(false);

  const drop = (e: DragEvent<HTMLDivElement>): void => {
    e.preventDefault();
    setOver(false);
    onFiles(Array.from(e.dataTransfer.files));
  };

  return (
    <div style={{ display: "grid", gap: 14 }}>
      {error !== undefined && <p role="alert" style={alertStyle}>{error}</p>}
      <div
        role="group"
        aria-label="Zona para soltar el archivo"
        onDragOver={(e) => { e.preventDefault(); setOver(true); }}
        onDragLeave={() => setOver(false)}
        onDrop={drop}
        style={{
          display: "grid", justifyItems: "center", gap: 12, padding: "32px 20px", textAlign: "center", borderRadius: "var(--r-xl)",
          border: `2px dashed ${over ? "var(--forest)" : "var(--border)"}`,
          background: over ? "color-mix(in oklab, var(--lime) 14%, var(--surface))" : "var(--surface-2)",
        }}
      >
        <span aria-hidden="true" style={{ color: "var(--forest)" }}><Icon name="upload" size={36} /></span>
        <p style={{ fontSize: 15.5, fontWeight: 600, color: "var(--text)", margin: 0 }}>Suelta aquí el Excel del sistema anterior</p>
        <input
          ref={input}
          aria-label="Archivo de Excel (.xlsx)"
          type="file"
          accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
          onChange={(e) => {
            onFiles(Array.from(e.target.files ?? []));
            e.target.value = "";
          }}
          style={{ position: "absolute", width: 1, height: 1, opacity: 0, overflow: "hidden" }}
          tabIndex={-1}
        />
        <Btn icon="upload" onClick={() => input.current?.click()}>Elegir archivo .xlsx</Btn>
        <p style={{ fontSize: 14, color: "var(--text-muted)", margin: 0 }}>Un archivo por nivel. Empieza con el más chico.</p>
      </div>
      <p aria-live="polite" style={{ fontSize: 14.5, color: "var(--text-muted)", margin: 0, minHeight: 20 }}>
        {reading !== undefined ? `Leyendo «${reading}»…` : ""}
      </p>
    </div>
  );
}
