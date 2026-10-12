"use client";

import type { ReactElement } from "react";

import { Icon } from "../../_design/Icon";
import { ModuleHeaderIcon } from "../../_design/ModuleHeaderIcon";

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
        <ModuleHeaderIcon icon="qr" color="lime" ink="var(--primary-ink)" />
        <div style={{ minWidth: 0 }}>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Asistencia</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>{subtitle}</p>
        </div>
      </div>
    </>
  );
}
