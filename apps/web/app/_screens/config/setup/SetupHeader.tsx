"use client";

import type { ReactElement } from "react";

import { Icon } from "../../../_design/Icon";
import { ModuleHeaderIcon } from "../../../_design/ModuleHeaderIcon";

/** "Volver al inicio", the screen's title, and a line naming the cycle and what is missing (§1.2). */
export function SetupHeader({ onBack, subtitle }: { onBack: () => void; subtitle: string | undefined }): ReactElement {
  return (
    <>
      <button
        onClick={onBack}
        className="fade-up"
        style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14 }}
      >
        <Icon name="back" size={18} stroke={2.4} />
        Volver al inicio
      </button>
      <div className="fade-up" style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 16, marginBottom: 24 }}>
        <ModuleHeaderIcon icon="settings" color="forest" shrinks />
        <div style={{ minWidth: 0 }}>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Configuración académica</h1>
          {subtitle !== undefined && <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>{subtitle}</p>}
        </div>
      </div>
    </>
  );
}
