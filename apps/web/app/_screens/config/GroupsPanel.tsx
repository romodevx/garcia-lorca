"use client";

import { type ReactElement } from "react";

import { InlineCreateRow } from "../../_design/InlineCreateRow";
import type { CreateGroupInput, Cycle, GradeLevel, Group } from "../../config/api";
import { CorrectableName, type NameCorrections } from "./CorrectableName";
import { useGroupCreateRow } from "./createRows";
import { DeletionStatus, useDeletionFeedback } from "./useDeletionFeedback";
import { EmptyHint, ErrorAlert, Section } from "./parts";

export interface GroupsPanelProps {
  cycle: Cycle;
  gradeLevel: GradeLevel;
  groups: readonly Group[];
  onCreate: (input: CreateGroupInput) => Promise<void>;
  /** config:write: rename / delete beside each group (T8). Absent = read-only pills. */
  corrections?: NameCorrections;
}

/**
 * Los salones de un grado en un ciclo: A, B, C…
 *
 * Es el alta más repetitiva de todo el catálogo —una escuela abre veinte grupos
 * en una tarde y luego no vuelve hasta el agosto siguiente— y por eso es donde
 * el cajón costaba más: veinte viajes para escribir veinte letras.
 *
 * Las dos celdas llegan propuestas, así que abrir el renglón y dar Enter crea el
 * grupo. Pero siguen siendo campos: el CUPO decide a qué grupo entra un alumno
 * nuevo, y un salón más chico que los demás tiene que poder capturarse.
 */
export function GroupsPanel({ cycle, gradeLevel, groups, onCreate, corrections }: GroupsPanelProps): ReactElement {
  const row = useGroupCreateRow(cycle, gradeLevel, groups, onCreate);
  const feedback = useDeletionFeedback();

  return (
    <Section title="Grupos" subtitle={`De ${gradeLevel.name} en ${cycle.name}.`}>
      {groups.length > 0 && corrections !== undefined && (
        <ul aria-label={`Grupos de ${gradeLevel.name}`} style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "start", listStyle: "none", padding: 0, margin: "0 0 16px" }}>
          {groups.map((g) => (
            <CorrectableName
              key={g.id}
              name={g.name}
              noun="grupo"
              onRename={(name) => corrections.rename(g.id, name)}
              onDelete={() => corrections.remove(g.id)}
              onDeleted={(next) => feedback.deleted(`Grupo ${g.name} borrado`, next)}
              deleteDescription={`Se borra el grupo ${g.name} de ${gradeLevel.name} en ${cycle.name}.`}
            >
              <GroupPill group={g} />
            </CorrectableName>
          ))}
        </ul>
      )}
      {groups.length > 0 && corrections === undefined && (
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginBottom: 16 }}>
          {groups.map((g) => <GroupPill key={g.id} group={g} />)}
        </div>
      )}
      {groups.length === 0 && (
        <EmptyHint>Aún no hay grupos para este grado en el ciclo.</EmptyHint>
      )}

      <DeletionStatus message={feedback.message} />
      <div ref={feedback.fallbackRef} style={{ border: "1px solid var(--border)", borderRadius: "var(--r-md)", overflow: "hidden" }}>
        <InlineCreateRow addLabel="Nuevo grupo" defaults={row.defaults} rules={row.rules} columns={row.columns} onCreate={row.onCreate} />
      </div>

      <p style={{ fontSize: 13, color: "var(--text-faint)", marginTop: 10, lineHeight: 1.5 }}>
        El <strong>cupo</strong> decide a qué grupo entra un alumno nuevo: se llena A y el
        siguiente pasa a B, sin que nadie lo asigne a mano.
      </p>

      {row.error !== undefined && <ErrorAlert message={row.error} />}
    </Section>
  );
}

/** "A · 30 lugares": the group as both setup views show it. */
export function GroupPill({ group }: { group: Group }): ReactElement {
  return (
    <span style={{ padding: "8px 14px", borderRadius: "var(--r-pill)", background: "var(--surface-2)", fontSize: 14, fontWeight: 600 }}>
      {group.name} · {group.capacity} lugares
    </span>
  );
}
