"use client";

import type { ReactElement } from "react";
import { Btn } from "../_design/Controls";
import { Icon } from "../_design/Icon";
import { Page } from "../_design/Page";
import { COLOR } from "../_design/colors";
import type { ModuleDef } from "../_design/data";

export interface ModulePlaceholderProps {
  m: ModuleDef;
  onBack: () => void;
}

/** Friendly stub so modules without a built surface never dead-end. */
export function ModulePlaceholder({ m, onBack }: ModulePlaceholderProps): ReactElement {
  const c = COLOR[m.color] ?? COLOR.neutral;
  const neutral = m.color === "neutral";
  return (
    <Page>
      <button
        onClick={onBack}
        className="fade-up"
        style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14 }}
      >
        <Icon name="back" size={18} stroke={2.4} />
        Volver al inicio
      </button>
      <div className="fade-up" style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 40 }}>
        <div
          style={{
            width: 56,
            height: 56,
            borderRadius: 18,
            display: "grid",
            placeItems: "center",
            background: neutral ? "var(--surface-3)" : c.v,
            color: neutral ? "var(--text-muted)" : c.ink
          }}
        >
          <Icon name={m.icon} size={30} />
        </div>
        <div>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>{m.label}</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>{m.desc}</p>
        </div>
      </div>
      <div
        className="fade-up"
        style={{
          display: "grid",
          placeItems: "center",
          padding: "60px 20px",
          textAlign: "center",
          background: "var(--surface)",
          border: "1px dashed var(--border-strong)",
          borderRadius: "var(--r-xl)"
        }}
      >
        <div
          style={{
            width: 96,
            height: 96,
            borderRadius: 28,
            display: "grid",
            placeItems: "center",
            marginBottom: 22,
            background: `color-mix(in oklab, ${c.v} 14%, var(--surface))`,
            color: neutral ? "var(--text-muted)" : c.v
          }}
        >
          <Icon name={m.icon} size={48} stroke={1.8} />
        </div>
        <h2 style={{ fontSize: 24, fontWeight: 700, color: "var(--text)" }}>Módulo «{m.label}»</h2>
        <p style={{ color: "var(--text-muted)", fontSize: 16, maxWidth: 440, marginTop: 10, lineHeight: 1.5 }}>
          Esta sección sigue el mismo patrón visual que{" "}
          <b style={{ color: "var(--forest)" }}>Finanzas</b>: lista clara, acciones de un clic y estados con color. Ábrela desde el panel
          para ver el detalle.
        </p>
        <div style={{ display: "flex", gap: 12, marginTop: 26 }}>
          <Btn icon="plus" kind={neutral ? "forest" : "primary"}>
            Crear nuevo
          </Btn>
          <Btn kind="ghost" onClick={onBack}>
            Volver
          </Btn>
        </div>
      </div>
    </Page>
  );
}
