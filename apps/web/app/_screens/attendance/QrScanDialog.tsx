"use client";

import { useState, type CSSProperties, type ReactElement } from "react";

import { DrawerShell } from "../../_design/DrawerShell";
import { Icon, type IconName } from "../../_design/Icon";
import { useQrCamera } from "./useQrCamera";

export type ScanTone = "ok" | "duplicate" | "error";

/** The result card shown over the camera; `id` changes on every scan so the same text re-flashes. */
export interface ScanFlash {
  id: number;
  tone: ScanTone;
  text: string;
}

export interface QrScanDialogProps {
  scannedCount: number;
  total: number;
  flash?: ScanFlash;
  /** The last result in words, for screen readers (aria-live polite). */
  announcement: string;
  onCode: (code: string) => void;
  onFinish: () => void;
}

const TONE: Record<ScanTone, { icon: IconName; bg: string; fg: string }> = {
  ok: { icon: "check", bg: "var(--forest)", fg: "#fff" },
  duplicate: { icon: "clock", bg: "var(--warn-bg)", fg: "var(--warn-fg)" },
  error: { icon: "warning", bg: "var(--red)", fg: "#fff" },
};

const FRAME_SIZE = "min(64vw, 280px)";
const SR_ONLY: CSSProperties = { position: "absolute", width: 1, height: 1, overflow: "hidden", clip: "rect(0 0 0 0)", whiteSpace: "nowrap" };
const PILL_BUTTON: CSSProperties = { minHeight: 44, padding: "0 18px", borderRadius: "var(--r-pill)", fontWeight: 800, fontSize: 15, cursor: "pointer" };

function FlashCard({ flash }: { flash: ScanFlash }): ReactElement {
  const tone = TONE[flash.tone];
  return (
    <div key={flash.id} data-testid="scan-flash" style={{ display: "flex", alignItems: "center", gap: 10, padding: "14px 18px", borderRadius: "var(--r-lg)", background: tone.bg, color: tone.fg, fontSize: 18, fontWeight: 800, boxShadow: "var(--shadow-pop)", animation: "pop .18s var(--ease) both", overflowWrap: "anywhere" }}>
      <Icon name={tone.icon} size={24} stroke={2.8} />
      <span>{flash.text}</span>
    </div>
  );
}

const TITLE = "Escanear credenciales";

/**
 * The camera, fullscreen on a phone: the teacher points it at one credential
 * after another without leaving it. Every result flashes a card (and sounds,
 * from the caller); "Terminar" or Esc closes it. The dialog contract (modal,
 * labelled, focus trap, focus return, portal, scroll lock) is the shared
 * DrawerShell's, in its `full` size.
 */
export function QrScanDialog({ scannedCount, total, flash, announcement, onCode, onFinish }: QrScanDialogProps): ReactElement {
  const [video, setVideo] = useState<HTMLVideoElement | null>(null);
  const camera = useQrCamera(video, onCode);

  const header = (
    <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 12, padding: "14px 16px", borderBottom: "1px solid var(--border)" }}>
      <div style={{ flex: "1 1 auto", minWidth: 0 }}>
        <h2 style={{ margin: 0, fontSize: 18, fontWeight: 800, color: "var(--text)" }}>{TITLE}</h2>
        <p style={{ margin: "2px 0 0", fontSize: 14, fontWeight: 700, color: "var(--forest)" }}>Escaneados {scannedCount} de {total}</p>
      </div>
      <button type="button" onClick={onFinish} style={{ ...PILL_BUTTON, background: "var(--primary)", color: "var(--primary-ink)" }}>Terminar</button>
    </div>
  );

  return (
    <DrawerShell title={TITLE} icon="qrScan" size="full" header={header} padded={false} onClose={onFinish}>
      <div style={{ position: "relative", flex: "1 1 auto", minHeight: 0, background: "#000", color: "#fff" }}>
        <video ref={setVideo} muted playsInline aria-hidden="true" style={{ width: "100%", height: "100%", objectFit: "cover", display: camera.status === "error" ? "none" : "block" }} />
        {camera.status !== "error" && (
          <div aria-hidden="true" style={{ position: "absolute", top: "50%", left: "50%", width: FRAME_SIZE, height: FRAME_SIZE, transform: "translate(-50%, -50%)", border: "3px solid var(--lime)", borderRadius: 18, boxShadow: "0 0 0 100vmax rgba(0,0,0,.35)" }} />
        )}
        {camera.status === "starting" && <p style={{ position: "absolute", top: 16, width: "100%", margin: 0, textAlign: "center", fontSize: 15 }}>Abriendo la cámara…</p>}
        {camera.status === "error" && (
          <div style={{ position: "absolute", inset: 0, display: "grid", placeContent: "center", gap: 16, padding: 24, textAlign: "center" }}>
            <p style={{ margin: 0, fontSize: 16, fontWeight: 600, lineHeight: 1.45 }}>{camera.error}</p>
            <button type="button" onClick={camera.retry} style={{ ...PILL_BUTTON, justifySelf: "center", background: "#fff", color: "#000" }}>Reintentar</button>
          </div>
        )}
        <div style={{ position: "absolute", left: 16, right: 16, bottom: 20 }}>{flash !== undefined && <FlashCard flash={flash} />}</div>
        <p role="status" aria-live="polite" style={SR_ONLY}>{announcement}</p>
      </div>
    </DrawerShell>
  );
}
