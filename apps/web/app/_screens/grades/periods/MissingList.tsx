"use client";

import type { ReactElement } from "react";

import type { MissingCapture } from "../../../grades/periods";

/** Missing captures grouped by group, in the order the server sent them. */
function byGroup(missing: readonly MissingCapture[]): { id: string; label: string; rows: MissingCapture[] }[] {
  const groups = new Map<string, { id: string; label: string; rows: MissingCapture[] }>();
  for (const m of missing) {
    const group = groups.get(m.groupId) ?? { id: m.groupId, label: m.groupLabel, rows: [] };
    group.rows.push(m);
    groups.set(m.groupId, group);
  }
  return [...groups.values()];
}

/** "Matemáticas · Ada Lovelace · faltan 30", under each group. */
export function MissingList({ missing }: { missing: readonly MissingCapture[] }): ReactElement {
  const total = missing.reduce((sum, m) => sum + m.missingCount, 0);
  return (
    <div style={{ display: "grid", gap: 12 }}>
      <p style={{ fontSize: 15, fontWeight: 700, color: "var(--warn-fg)" }}>
        {total === 1 ? "Falta 1 calificación por capturar" : `Faltan ${total} calificaciones por capturar`}
      </p>
      {byGroup(missing).map((g) => (
        <div key={g.id} role="group" aria-label={g.label} style={{ padding: "10px 14px", borderRadius: "var(--r-md)", background: "var(--surface-2)" }}>
          <p style={{ fontSize: 14, fontWeight: 700, color: "var(--text)", marginBottom: 4 }}>{g.label}</p>
          <ul style={{ listStyle: "none", padding: 0, margin: 0, display: "grid", gap: 2 }}>
            {g.rows.map((m) => (
              <li key={m.subjectId} style={{ fontSize: 14, color: "var(--text-muted)" }}>
                {`${m.subjectName} · ${m.teacherName} · faltan ${m.missingCount}`}
              </li>
            ))}
          </ul>
        </div>
      ))}
    </div>
  );
}
