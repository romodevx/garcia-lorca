"use client";

import { useEffect, type CSSProperties, type FormEvent, type ReactElement, type ReactNode } from "react";

import { Btn } from "./Controls";
import { Icon, type IconName } from "./Icon";

/**
 * The right-hand drawer every "create one of these" flow uses.
 *
 * It exists because the pattern was already in the app three times — a new
 * pupil, a payment at the counter, a student's file — written out by hand each
 * time. The academic catalogue was the surface that had NOT got it: it put its
 * forms inline, in panels that grew as you selected things, and the result was
 * a screen the school called confusing. One drawer, one shape, everywhere.
 *
 * A real <form>, so Enter saves and the browser does the focus work. Escape and
 * the backdrop close it; a click inside never does, because losing a half-typed
 * form to a stray click is the fastest way to make somebody stop using a screen.
 */

const closeButton: CSSProperties = {
  width: 40, height: 40, borderRadius: "50%", display: "grid", placeItems: "center",
  background: "var(--surface-3)", border: "1px solid var(--border)",
  color: "var(--text-muted)", cursor: "pointer",
};

export interface SlideOverProps {
  title: string;
  subtitle?: string;
  icon: IconName;
  /** What the primary button says. "Crear nivel" beats "Guardar". */
  submitLabel: string;
  /** True while the save is in flight: the button says so and stops accepting clicks. */
  pending?: boolean;
  /** Why the last save was refused, in words the person can act on. */
  error?: string;
  onClose: () => void;
  onSubmit: () => void;
  children: ReactNode;
}

export function SlideOver({
  title, subtitle, icon, submitLabel, pending = false, error, onClose, onSubmit, children,
}: SlideOverProps): ReactElement {
  useEffect(() => {
    const onKey = (e: KeyboardEvent): void => { if (e.key === "Escape") { onClose(); } };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const submit = (e: FormEvent): void => {
    e.preventDefault();
    if (!pending) {
      onSubmit();
    }
  };

  return (
    <div
      data-testid="slideover-backdrop"
      onClick={onClose}
      style={{ position: "fixed", inset: 0, zIndex: 70, background: "rgba(6,20,12,.5)", backdropFilter: "blur(4px)", display: "flex", justifyContent: "flex-end" }}>
      <form
        onClick={(e) => e.stopPropagation()}
        onSubmit={submit}
        role="dialog"
        aria-label={title}
        style={{ width: "min(520px, 100%)", height: "100%", background: "var(--surface)", borderLeft: "1px solid var(--border)", boxShadow: "var(--shadow-pop)", display: "flex", flexDirection: "column", animation: "slideIn .28s var(--ease) both" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 14, padding: "22px 26px", borderBottom: "1px solid var(--border)" }}>
          <div style={{ width: 48, height: 48, borderRadius: 14, display: "grid", placeItems: "center", background: "var(--forest)", color: "#fff" }}>
            <Icon name={icon} size={26} />
          </div>
          <div style={{ flex: 1 }}>
            <h2 style={{ fontSize: 21, fontWeight: 700, color: "var(--text)" }}>{title}</h2>
            {subtitle !== undefined && (
              <p style={{ fontSize: 13.5, color: "var(--text-muted)", marginTop: 2 }}>{subtitle}</p>
            )}
          </div>
          <button type="button" onClick={onClose} aria-label="Cerrar" style={closeButton}>
            <Icon name="close" size={19} />
          </button>
        </div>

        <div style={{ flex: 1, overflowY: "auto", padding: "22px 26px", display: "flex", flexDirection: "column", gap: 18 }}>
          {children}
          {error !== undefined && (
            <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14, margin: 0 }}>{error}</p>
          )}
        </div>

        <div style={{ display: "flex", gap: 12, alignItems: "center", padding: "18px 26px", borderTop: "1px solid var(--border)", background: "var(--surface-2)" }}>
          <Btn kind="ghost" type="button" onClick={onClose}>Cancelar</Btn>
          <div style={{ flex: 1 }} />
          {/* Disabled ONLY while the request is in flight. An incomplete form
              gets a sentence back, never a dead control that says nothing. */}
          <Btn icon="check" type="submit" disabled={pending} style={{ opacity: pending ? 0.45 : 1, height: 52, fontSize: 16 }}>
            {pending ? "Guardando…" : submitLabel}
          </Btn>
        </div>
      </form>
    </div>
  );
}
