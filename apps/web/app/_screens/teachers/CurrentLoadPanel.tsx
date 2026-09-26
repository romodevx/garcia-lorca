"use client";

import { useId, type ReactElement } from "react";

import { Icon } from "../../_design/Icon";
import type { LoadSlot, SubjectLoad } from "../../teachers/load";
import { EmptyHint, Section } from "../config/parts";

export interface CurrentLoadPanelProps {
  load: readonly SubjectLoad[];
  /** Frees a slot; absent for a read-only viewer, who gets no ×. */
  onRemove?: (slot: LoadSlot) => void;
}

/**
 * What the teacher already teaches, one card per subject name with its groups
 * as chips. The × frees the slot at once; the page offers Deshacer instead of
 * a confirmation (a removed class is not irreversible).
 */
export function CurrentLoadPanel({ load, onRemove }: CurrentLoadPanelProps): ReactElement {
  const groupCount = load.reduce((n, s) => n + s.slots.length, 0);
  const subtitle =
    load.length === 0 ? "Asignaturas y grupos que imparte." : `${load.length} ${plural(load.length, "asignatura", "asignaturas")} · ${groupCount} ${plural(groupCount, "grupo", "grupos")}`;

  return (
    <Section title="Carga actual" subtitle={subtitle}>
      {load.length === 0 ? (
        <EmptyHint>Aún no tiene asignaturas en este ciclo. Agrégalas abajo.</EmptyHint>
      ) : (
        <div style={{ display: "grid", gap: 12 }}>
          {load.map((subject) => (
            <SubjectCard key={subject.subjectName} subject={subject} onRemove={onRemove} />
          ))}
        </div>
      )}
    </Section>
  );
}

function SubjectCard({ subject, onRemove }: { subject: SubjectLoad; onRemove?: (slot: LoadSlot) => void }): ReactElement {
  const titleId = useId();
  return (
    <section
      aria-labelledby={titleId}
      style={{ padding: "12px 14px", borderRadius: "var(--r-lg)", background: "var(--surface-2)", border: "1px solid var(--border)" }}
    >
      <h3 id={titleId} style={{ fontSize: 15.5, fontWeight: 700, color: "var(--text)", marginBottom: 8 }}>
        {subject.subjectName}
      </h3>
      <ul style={{ listStyle: "none", padding: 0, margin: 0, display: "flex", flexWrap: "wrap", gap: 8 }}>
        {subject.slots.map((slot) => (
          <li
            key={slot.assignmentId}
            style={{ display: "inline-flex", alignItems: "center", gap: 2, minHeight: 44, paddingLeft: 14, paddingRight: onRemove === undefined ? 14 : 0, borderRadius: "var(--r-pill)", background: "var(--surface)", border: "1px solid var(--border)", fontSize: 14, fontWeight: 600, color: "var(--text)", maxWidth: "100%" }}
          >
            <span style={{ overflowWrap: "anywhere" }}>{slot.groupLabel}</span>
            {onRemove !== undefined && (
            <button
              type="button"
              onClick={() => onRemove(slot)}
              aria-label={`Quitar ${slot.subjectName} de ${slot.groupLabel}`}
              style={{ width: 44, height: 44, flexShrink: 0, display: "grid", placeItems: "center", borderRadius: "var(--r-pill)", color: "var(--text-muted)", cursor: "pointer" }}
            >
              <Icon name="close" size={16} stroke={2.4} />
            </button>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

function plural(n: number, one: string, many: string): string {
  return n === 1 ? one : many;
}
