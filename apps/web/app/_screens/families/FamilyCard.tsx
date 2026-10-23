"use client";

import { useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { formatStudentName } from "../../_design/students/studentName";
import type { FamiliesAccess } from "../../families/model";
import { FamilyJoinPicker } from "./FamilyJoinPicker";
import { useFamilyDetail } from "./useFamilyDetail";

export interface FamilyCardProps {
  studentId: string;
  /** The pupil's family as the file was read; absent when it has none. */
  familyId: string | undefined;
  access: FamiliesAccess;
  onOpenFamily: (familyId: string) => void;
  onOpenStudent: (studentId: string) => void;
}

const MUTED = { fontSize: 14, color: "var(--text-muted)", margin: 0 } as const;

/** The "Familia" card of a pupil's file (ADR 0041 D1). */
export function FamilyCard({ studentId, familyId, access, onOpenFamily, onOpenStudent }: FamilyCardProps): ReactElement {
  const [current, setCurrent] = useState(familyId);
  const [joining, setJoining] = useState(false);

  if (current !== undefined) {
    return <FamilyOfPupil familyId={current} studentId={studentId} onOpenFamily={onOpenFamily} onOpenStudent={onOpenStudent} />;
  }
  if (!access.canWriteGuardians) return <p style={MUTED}>Sin familia.</p>;
  if (!joining) {
    return (
      <div style={{ display: "grid", gap: 8 }}>
        <p style={MUTED}>Todavía no está en una familia.</p>
        <div><Btn kind="ghost" icon="students" onClick={() => setJoining(true)}>Agregar a una familia</Btn></div>
      </div>
    );
  }
  return <FamilyJoinPicker studentId={studentId} onJoined={setCurrent} />;
}

function FamilyOfPupil({ familyId, studentId, onOpenFamily, onOpenStudent }: { familyId: string; studentId: string; onOpenFamily: (id: string) => void; onOpenStudent: (id: string) => void }): ReactElement {
  const { detail, status } = useFamilyDetail(familyId, false);
  if (status === "loading") return <p style={MUTED}>Cargando…</p>;
  if (detail === undefined) return <p role="alert" style={{ ...MUTED, color: "var(--bad-fg)" }}>No pudimos leer su familia.</p>;
  const siblings = detail.members.filter((m) => m.id !== studentId);

  return (
    <div style={{ display: "grid", gap: 8 }}>
      <p style={{ fontSize: 16, fontWeight: 700, color: "var(--text)", margin: 0 }}>{detail.name}</p>
      {siblings.length === 0 && <p style={MUTED}>Sin hermanos en la escuela.</p>}
      {siblings.length > 0 && (
        <ul aria-label="Hermanos" style={{ display: "flex", flexWrap: "wrap", gap: 8, listStyle: "none", margin: 0, padding: 0 }}>
          {siblings.map((m) => (
            <li key={m.id}>
              <button type="button" onClick={() => onOpenStudent(m.id)} style={{ minHeight: 44, fontSize: 14.5, fontWeight: 600, color: "var(--forest)", textDecoration: "underline", cursor: "pointer", background: "transparent" }}>
                {formatStudentName(m)}
              </button>
            </li>
          ))}
        </ul>
      )}
      <div><Btn kind="ghost" icon="chevron" onClick={() => onOpenFamily(familyId)}>Ver familia</Btn></div>
    </div>
  );
}
