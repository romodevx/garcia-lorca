"use client";

import { useMemo, useState, type CSSProperties, type ReactElement } from "react";
import { Icon } from "../../_design/Icon";
import type { CollectStudent } from "./CollectDrawer";

const overlay: CSSProperties = {
  position: "fixed", inset: 0, zIndex: 80, background: "rgba(6,20,12,.5)", backdropFilter: "blur(4px)",
  display: "flex", alignItems: "flex-start", justifyContent: "center", paddingTop: "12vh",
};
const panel: CSSProperties = {
  width: "min(560px, 92vw)", background: "var(--surface)", border: "1px solid var(--border)",
  borderRadius: "var(--r-xl)", boxShadow: "var(--shadow-pop)", overflow: "hidden",
};
const search: CSSProperties = {
  width: "100%", height: 60, paddingLeft: 52, paddingRight: 18, border: "none",
  background: "transparent", color: "var(--text)", fontSize: 18, outline: "none",
};

export interface StudentPickerProps {
  students: CollectStudent[];
  onPick: (student: CollectStudent) => void;
  onClose: () => void;
}

/**
 * Find the pupil, then take the money.
 *
 * The counter's entry point is a person, not a charge. A cashier is handed cash
 * by a parent who says a name; making them find the right row of the right month
 * first is what sent the office back to the notebook.
 *
 * At most ten names are shown. A list longer than that is not read, it is
 * scrolled — and the answer is to type another letter.
 */
export function StudentPicker({ students, onPick, onClose }: StudentPickerProps): ReactElement {
  const [q, setQ] = useState("");

  const matches = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return students
      .filter((s) => needle === "" || `${s.firstName} ${s.lastName}`.toLowerCase().includes(needle))
      .slice(0, 10);
  }, [students, q]);

  return (
    <div
      style={overlay}
      onClick={onClose}
      onKeyDown={(e) => { if (e.key === "Escape") { onClose(); } }}>
      <div role="dialog" aria-label="Buscar alumno para cobrar" style={panel} onClick={(e) => e.stopPropagation()}>
        <div style={{ position: "relative", borderBottom: "1px solid var(--border)" }}>
          <span style={{ position: "absolute", left: 20, top: "50%", transform: "translateY(-50%)", color: "var(--text-faint)" }}>
            <Icon name="search" size={21} />
          </span>
          <input autoFocus value={q} onChange={(e) => setQ(e.target.value)} aria-label="Buscar alumno" placeholder="Nombre del alumno…" style={search} />
        </div>
        <div style={{ maxHeight: "48vh", overflowY: "auto" }}>
          {matches.length === 0 && (
            <p style={{ padding: "22px 24px", margin: 0, color: "var(--text-muted)", fontSize: 15 }}>
              Ningún alumno con ese nombre.
            </p>
          )}
          {matches.map((s) => (
            <button
              key={s.id}
              onClick={() => onPick(s)}
              style={{ display: "flex", width: "100%", alignItems: "center", gap: 12, padding: "14px 24px", background: "transparent", border: "none", borderTop: "1px solid var(--border)", cursor: "pointer", textAlign: "left" }}>
              <span style={{ fontSize: 15.5, fontWeight: 600, color: "var(--text)" }}>{s.firstName} {s.lastName}</span>
              <span style={{ fontSize: 13.5, color: "var(--text-muted)" }}>{s.gradeGroup ?? ""}</span>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
