"use client";

import { useId, useMemo, useState, type KeyboardEvent, type ReactElement } from "react";

import { matchesSearch } from "../../_design/textSearch";
import { formatStudentName } from "../../_design/students/studentName";
import type { Family } from "../../families/api";
import type { Student } from "../../students/api";
import { FAMILY_LIMIT, familyName } from "./audience";
import { TargetRow } from "./TargetRow";

const LIMIT_NOTE = `Un aviso llega a ${FAMILY_LIMIT} familias como máximo. Para más, usa Grupos, Niveles o Toda la escuela.`;

export interface FamilySearchProps {
  families: readonly Family[];
  /** The pupils, to name each family's children. */
  students: readonly Student[];
  picked: readonly string[];
  onToggle: (id: string) => void;
  loaded: boolean;
  failed: boolean;
  atLimit: boolean;
}

interface FamilyEntry {
  id: string;
  label: string;
  name: string;
  children: string;
}

function keepEnterFromSubmitting(e: KeyboardEvent<HTMLInputElement>): void {
  if (e.key === "Enter") e.preventDefault();
}

/**
 * "Familias" (ADR 0041): several at once, like groups, found by the family's
 * name or any child's. Each row names the children, so two "GARCÍA" families
 * are told apart.
 */
export function FamilySearch({ families, students, picked, onToggle, loaded, failed, atLimit }: FamilySearchProps): ReactElement {
  const inputId = useId();
  const [query, setQuery] = useState("");
  const entries = useMemo((): FamilyEntry[] => {
    const nameOf = new Map(students.map((s) => [s.id, formatStudentName(s)]));
    return families.map((f) => ({ id: f.id, label: familyName(f), name: f.name, children: f.studentIds.flatMap((id) => nameOf.get(id) ?? []).join(", ") }));
  }, [families, students]);
  const shown = entries.filter((e) => matchesSearch(query, e.name, e.children));

  if (failed) return <p role="alert" className="announce-note">No pudimos leer las familias.</p>;
  if (!loaded) return <p className="announce-note">Cargando familias…</p>;
  if (entries.length === 0) return <p className="announce-note">Todavía no hay familias. Se arman en Alumnos › Familias.</p>;
  return (
    <div className="announce-search">
      <label htmlFor={inputId} className="announce-search__label">Buscar familia</label>
      <input id={inputId} type="search" className="announce-search__input" autoComplete="off" value={query} onChange={(e) => setQuery(e.target.value)} onKeyDown={keepEnterFromSubmitting} />
      <div className="announce-rows">
        {shown.map((e) => (
          <TargetRow key={e.id} label={e.label} text={e.name} detail={e.children === "" ? undefined : e.children} on={picked.includes(e.id)} onToggle={() => onToggle(e.id)} mark="check" />
        ))}
      </div>
      {query.trim() !== "" && shown.length === 0 && <p className="announce-note">Ninguna familia coincide con «{query.trim()}».</p>}
      {atLimit && <p role="status" className="announce-note">{LIMIT_NOTE}</p>}
    </div>
  );
}
