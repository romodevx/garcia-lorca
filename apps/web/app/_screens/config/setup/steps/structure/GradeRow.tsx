"use client";

import { useId, useRef, type CSSProperties, type ReactElement } from "react";

import { InlineCreateRow } from "../../../../../_design/InlineCreateRow";
import type { CreateGroupInput, Cycle, GradeLevel, Group } from "../../../../../config/api";
import { CorrectableName, type NameCorrections } from "../../../CorrectableName";
import { useGroupCreateRow } from "../../../createRows";
import { GroupPill } from "../../../GroupsPanel";
import { DeletionStatus, useDeletionFeedback } from "../../../useDeletionFeedback";
import { ErrorAlert } from "../../../parts";

export interface GradeRowProps {
  grade: GradeLevel;
  levelName: string;
  /** The grade's groups in the selected cycle; undefined while they cannot be told (loading or failed). */
  groups: readonly Group[] | undefined;
  /** Undefined without a cycle: nothing to create groups in. */
  cycle: Cycle | undefined;
  createGroup: (input: CreateGroupInput) => Promise<void>;
  gradeCorrections: NameCorrections | undefined;
  groupCorrections: NameCorrections | undefined;
  /** The grade was deleted: say so and put the focus where the card decides. */
  onGradeDeleted: (message: string, next: HTMLElement | null) => void;
}

const rowStyle: CSSProperties = {
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: "8px 12px",
  padding: "10px 0",
  borderTop: "1px solid var(--border)",
};

const inlineList: CSSProperties = { display: "flex", flexWrap: "wrap", alignItems: "center", gap: 8, listStyle: "none", padding: 0, margin: 0 };

/**
 * One grade of a level card: its name (rename / delete beside it, as in
 * «Niveles y grados»), its groups as pills (each with its own rename / delete)
 * and «Agregar grupo a …», the same create row as «Grupos». On a phone the
 * name sits on its own line and the pills wrap under it.
 */
export function GradeRow({ grade, levelName, groups, cycle, createGroup, gradeCorrections, groupCorrections, onGradeDeleted }: GradeRowProps): ReactElement {
  const nameId = useId();
  const rowRef = useRef<HTMLLIElement>(null);
  const feedback = useDeletionFeedback();
  const known = groups ?? [];
  const row = useGroupCreateRow(cycle ?? NO_CYCLE, grade, known, createGroup);

  // The next grade's rename control, else the previous one's, read before the row goes.
  const neighbourGrade = (): HTMLElement | null => {
    const item = rowRef.current;
    const pick = (el: Element | null | undefined) => el?.querySelector<HTMLElement>("[data-correct-rename]") ?? null;
    return pick(item?.nextElementSibling) ?? pick(item?.previousElementSibling);
  };

  return (
    <li ref={rowRef} aria-labelledby={nameId} style={rowStyle}>
      <div style={{ minWidth: 120, flex: "0 0 auto" }}>
        <span id={nameId} hidden>{grade.name}</span>
        {gradeCorrections !== undefined ? (
          <ul style={inlineList}>
            <CorrectableName
              name={grade.name}
              noun="grado"
              onRename={(name) => gradeCorrections.rename(grade.id, name)}
              onDelete={() => gradeCorrections.remove(grade.id)}
              onDeleted={() => onGradeDeleted(`Grado ${grade.name} borrado`, neighbourGrade())}
              deleteDescription={`Se borra el grado ${grade.name} de ${levelName}. El nivel y sus otros grados no cambian.`}
            >
              <GradeName name={grade.name} />
            </CorrectableName>
          </ul>
        ) : (
          <GradeName name={grade.name} />
        )}
      </div>

      <div style={{ flex: "1 1 260px", minWidth: 0, display: "flex", flexWrap: "wrap", alignItems: "center", gap: 8 }}>
        {groups !== undefined && groups.length === 0 && (
          <span style={{ fontSize: 14, fontWeight: 600, color: "var(--warn-fg)" }}>Sin grupos</span>
        )}
        {known.length > 0 && groupCorrections !== undefined && (
          <ul aria-label={`Grupos de ${grade.name}`} style={inlineList}>
            {known.map((g) => (
              <CorrectableName
                key={g.id}
                name={g.name}
                noun="grupo"
                onRename={(name) => groupCorrections.rename(g.id, name)}
                onDelete={() => groupCorrections.remove(g.id)}
                onDeleted={(next) => feedback.deleted(`Grupo ${g.name} borrado`, next)}
                deleteDescription={`Se borra el grupo ${g.name} de ${grade.name}${cycle === undefined ? "" : ` en ${cycle.name}`}.`}
              >
                <GroupPill group={g} />
              </CorrectableName>
            ))}
          </ul>
        )}
        {known.length > 0 && groupCorrections === undefined && (
          <ul aria-label={`Grupos de ${grade.name}`} style={inlineList}>
            {known.map((g) => <li key={g.id}><GroupPill group={g} /></li>)}
          </ul>
        )}
        <DeletionStatus message={feedback.message} />
        {cycle !== undefined && groups !== undefined && (
          <div ref={feedback.fallbackRef} style={{ flex: "1 1 260px", minWidth: 0 }}>
            <InlineCreateRow addLabel={`Agregar grupo a ${grade.name}`} defaults={row.defaults} rules={row.rules} columns={row.columns} onCreate={row.onCreate} />
          </div>
        )}
      </div>
      {row.error !== undefined && <div style={{ flexBasis: "100%" }}><ErrorAlert message={row.error} /></div>}
    </li>
  );
}

/** A placeholder the create row never sends: without a cycle it is not drawn. */
const NO_CYCLE: Cycle = { id: "", name: "", status: "draft", startsOn: "", endsOn: "" };

function GradeName({ name }: { name: string }): ReactElement {
  return <span style={{ fontSize: 16.5, fontWeight: 700, color: "var(--text)" }}>{name}</span>;
}
