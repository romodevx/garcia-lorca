"use client";

import { useId, type ReactElement } from "react";

import { Icon } from "../../_design/Icon";
import type { ScholarshipTarget } from "../../scholarships/api";
import { groupTargets } from "../../scholarships/format";

export interface TargetChecklistProps {
  targets: readonly ScholarshipTarget[];
  chosen: readonly string[];
  onChange: (next: string[]) => void;
  error?: string;
}

const legendStyle = { fontSize: 13, fontWeight: 600, color: "var(--text-muted)", marginBottom: 6 } as const;

/**
 * What a scholarship applies to (T18), grouped Inscripción / Colegiatura /
 * Conceptos / Actividades. Native checkboxes: Space toggles, Tab walks them.
 * Chosen keys keep the order they were ticked in.
 */
export function TargetChecklist({ targets, chosen, onChange, error }: TargetChecklistProps): ReactElement {
  const errorId = useId();
  const picked = new Set(chosen);
  const toggle = (key: string): void => onChange(picked.has(key) ? chosen.filter((k) => k !== key) : [...chosen, key]);

  return (
    <fieldset aria-describedby={error !== undefined ? errorId : undefined} style={{ border: "none", padding: 0, margin: 0, minWidth: 0 }}>
      {/* No hand-drawn asterisk (adoption.test): a missing choice is said on save. */}
      <legend style={legendStyle}>Aplica a</legend>
      {targets.length === 0 && <p style={{ margin: 0, fontSize: 14, color: "var(--text-muted)" }}>No hay nada que becar todavía.</p>}
      <div style={{ display: "grid", gap: 12 }}>
        {groupTargets(targets).map((g) => (
          <div key={g.group} style={{ display: "grid", gap: 2 }}>
            <h3 style={{ fontSize: 12.5, fontWeight: 700, color: "var(--text-faint)", textTransform: "uppercase", letterSpacing: ".04em" }}>{g.label}</h3>
            {g.targets.map((t) => (
              <label key={t.key} style={{ display: "flex", alignItems: "center", gap: 10, minHeight: 44, cursor: "pointer", fontSize: 15, color: "var(--text)" }}>
                <input type="checkbox" checked={picked.has(t.key)} onChange={() => toggle(t.key)}
                  aria-invalid={error !== undefined ? true : undefined}
                  style={{ width: 20, height: 20, accentColor: "var(--forest)", flexShrink: 0 }} />
                {t.label}
              </label>
            ))}
          </div>
        ))}
      </div>
      {error !== undefined && (
        <p id={errorId} role="alert" style={{ display: "flex", alignItems: "center", gap: 5, fontSize: 12.5, fontWeight: 600, color: "var(--bad-fg)", marginTop: 5 }}>
          <Icon name="warning" size={14} stroke={2.4} />
          {error}
        </p>
      )}
    </fieldset>
  );
}
