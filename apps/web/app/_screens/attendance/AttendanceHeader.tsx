"use client";

import type { ReactElement } from "react";

import { Icon } from "../../_design/Icon";

/** "Volver al inicio" + the module heading, shared by the teacher and director surfaces. */
export function AttendanceHeader({ subtitle, onBack }: { subtitle: string; onBack: () => void }): ReactElement {
  return (
    <>
      <button
        type="button"
        onClick={onBack}
        className="fade-up"
        style={{ display: "inline-flex", alignItems: "center", gap: 7, minHeight: 44, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 6, cursor: "pointer" }}
      >
        <Icon name="back" size={18} stroke={2.4} />
        Volver al inicio
      </button>
      <div className="fade-up" style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 20 }}>
        <div style={{ width: 56, height: 56, flexShrink: 0, borderRadius: 18, display: "grid", placeItems: "center", background: "var(--lime)", color: "var(--primary-ink)", boxShadow: "0 6px 16px color-mix(in oklab, var(--lime) 40%, transparent)" }}>
          <Icon name="qr" size={30} />
        </div>
        <div style={{ minWidth: 0 }}>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Asistencia</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>{subtitle}</p>
        </div>
      </div>
    </>
  );
}
