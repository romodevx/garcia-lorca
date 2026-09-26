"use client";

import { useEffect, useId, useRef, useState, type CSSProperties, type ReactElement } from "react";
import { createPortal } from "react-dom";

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

/** APG dialog focus: enter on the first control, stay inside, return to the trigger on close. */
function useDialogFocus(panelRef: React.RefObject<HTMLDivElement | null>, onEscape: () => void, mounted: boolean): void {
  const returnTo = useRef<HTMLElement | null>(null);
  if (returnTo.current === null && typeof document !== "undefined") returnTo.current = document.activeElement as HTMLElement | null;
  const escape = useRef(onEscape);
  escape.current = onEscape;

  useEffect(() => {
    if (!mounted) return;
    const focusables = (): HTMLElement[] => Array.from(panelRef.current?.querySelectorAll<HTMLElement>("button:not([disabled])") ?? []);
    focusables()[0]?.focus();
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === "Escape") {
        e.preventDefault();
        escape.current();
        return;
      }
      if (e.key !== "Tab") return;
      const list = focusables();
      const first = list[0];
      const last = list[list.length - 1];
      if (first === undefined || last === undefined) return;
      const inside = panelRef.current?.contains(document.activeElement) === true;
      if (!inside || document.activeElement === (e.shiftKey ? first : last)) {
        e.preventDefault();
        (e.shiftKey ? last : first).focus();
      }
    };
    window.addEventListener("keydown", onKey);
    const trigger = returnTo.current;
    return () => {
      window.removeEventListener("keydown", onKey);
      trigger?.focus();
    };
  }, [mounted, panelRef]);
}

function FlashCard({ flash }: { flash: ScanFlash }): ReactElement {
  const tone = TONE[flash.tone];
  return (
    <div key={flash.id} data-testid="scan-flash" style={{ display: "flex", alignItems: "center", gap: 10, padding: "14px 18px", borderRadius: "var(--r-lg)", background: tone.bg, color: tone.fg, fontSize: 18, fontWeight: 800, boxShadow: "var(--shadow-pop)", animation: "pop .18s var(--ease) both", overflowWrap: "anywhere" }}>
      <Icon name={tone.icon} size={24} stroke={2.8} />
      <span>{flash.text}</span>
    </div>
  );
}

/**
 * The camera, fullscreen on a phone: the teacher points it at one credential
 * after another without leaving it. Every result flashes a card (and sounds,
 * from the caller); "Terminar" or Esc closes it.
 */
export function QrScanDialog({ scannedCount, total, flash, announcement, onCode, onFinish }: QrScanDialogProps): ReactElement | null {
  const titleId = useId();
  const panelRef = useRef<HTMLDivElement | null>(null);
  const [video, setVideo] = useState<HTMLVideoElement | null>(null);
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);
  useDialogFocus(panelRef, onFinish, mounted);
  const camera = useQrCamera(video, onCode);

  useEffect(() => {
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previous;
    };
  }, []);

  const panel = (
    <div style={{ position: "fixed", inset: 0, zIndex: 80, display: "grid", placeItems: "center", background: "rgba(6,20,12,.7)" }}>
      <div ref={panelRef} role="dialog" aria-modal="true" aria-labelledby={titleId} style={{ width: "min(100%, 560px)", height: "min(100%, 860px)", display: "flex", flexDirection: "column", background: "#000", color: "#fff", overflow: "hidden", borderRadius: 0 }}>
        <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 12, padding: "14px 16px", background: "var(--surface)", color: "var(--text)", borderBottom: "1px solid var(--border)" }}>
          <div style={{ flex: "1 1 auto", minWidth: 0 }}>
            <h2 id={titleId} style={{ margin: 0, fontSize: 18, fontWeight: 800 }}>Escanear credenciales</h2>
            <p style={{ margin: "2px 0 0", fontSize: 14, fontWeight: 700, color: "var(--forest)" }}>Escaneados {scannedCount} de {total}</p>
          </div>
          <button type="button" onClick={onFinish} style={{ ...PILL_BUTTON, background: "var(--primary)", color: "var(--primary-ink)" }}>Terminar</button>
        </div>

        <div style={{ position: "relative", flex: "1 1 auto", minHeight: 0 }}>
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
        </div>
        <p role="status" aria-live="polite" style={SR_ONLY}>{announcement}</p>
      </div>
    </div>
  );

  return mounted ? createPortal(panel, document.body) : null;
}
