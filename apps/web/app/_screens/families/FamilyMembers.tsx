"use client";

import type { ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { formatStudentName } from "../../_design/students/studentName";
import type { FamilyMember } from "../../families/api";
import type { FamilyPupil } from "./familyPupil";
import { PupilSearch } from "./PupilSearch";

const SECTION_TITLE = { fontSize: 13.5, fontWeight: 700, color: "var(--text-muted)", textTransform: "uppercase", letterSpacing: ".04em" } as const;

export interface MemberActions {
  onRemove: (member: FamilyMember) => void;
  onAdd: (pupil: FamilyPupil) => void;
  /** Pupils without a family, offered by "Agregar hijo". */
  candidates: readonly FamilyPupil[];
  addError?: string;
}

/** The children: each opens their own file; guardians:write may take one out or add one. */
export function FamilyMembers({ members, pupils, onOpenStudent, actions }: {
  members: readonly FamilyMember[];
  pupils: ReadonlyMap<string, FamilyPupil>;
  onOpenStudent: (studentId: string) => void;
  /** Absent without guardians:write. */
  actions?: MemberActions;
}): ReactElement {
  return (
    <section aria-label="Hijos" style={{ display: "grid", gap: 10 }}>
      <h3 style={SECTION_TITLE}>Hijos</h3>
      <ul style={{ display: "grid", gap: 0, listStyle: "none", margin: 0, padding: 0 }}>
        {members.map((m) => {
          const name = formatStudentName(m);
          return (
            <li key={m.id} style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap", padding: "8px 0", borderBottom: "1px solid var(--border)" }}>
              <div style={{ flex: 1, minWidth: 0 }}>
                <button type="button" onClick={() => onOpenStudent(m.id)} style={{ minHeight: 44, fontSize: 15, fontWeight: 600, color: "var(--forest)", textDecoration: "underline", textAlign: "left", cursor: "pointer", background: "transparent" }}>{name}</button>
                <div style={{ fontSize: 13, color: "var(--text-muted)" }}>
                  {pupils.get(m.id)?.room ?? `${m.level} · ${m.gradeGroup}`}
                  {m.draft && " · Borrador: no recibe las cuentas de la familia"}
                </div>
              </div>
              {actions !== undefined && (
                <Btn kind="ghost" onClick={() => actions.onRemove(m)} aria-label={`Quitar a ${name} de la familia`} style={{ height: 44, padding: "0 14px", fontSize: 14 }}>Quitar</Btn>
              )}
            </li>
          );
        })}
      </ul>
      {actions !== undefined && (
        <>
          <PupilSearch label="Agregar hijo" candidates={actions.candidates} onPick={actions.onAdd} />
          {actions.addError !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 13.5, margin: 0 }}>{actions.addError}</p>}
        </>
      )}
    </section>
  );
}
