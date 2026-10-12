"use client";

import { useId, useMemo, useState, type KeyboardEvent, type ReactElement } from "react";
import { matchesSearch } from "../../_design/textSearch";
import { formatStudentName, sortStudents, studentNameSearchKeys } from "../../_design/students/studentName";
import type { Student } from "../../students/api";
import { TargetRow } from "./TargetRow";

/** How many results the column shows before it asks to keep typing (D10). */
const MAX_RESULTS = 8;

export interface StudentSearchProps {
  students: Student[];
  picked: string[];
  onToggle: (id: string) => void;
  /** False until the pupils' load settles (D11). */
  loaded: boolean;
}

interface PupilEntry {
  id: string;
  name: string;
  detail: string;
  keys: readonly string[];
}

/** "Primaria · 1° A": `gradeGroup` alone may be just the letter. */
function groupLine(s: Student): string {
  return [s.level, s.gradeGroup].filter((part) => part !== "").join(" · ");
}

function toEntries(students: readonly Student[]): PupilEntry[] {
  return sortStudents(students).map((s) => ({ id: s.id, name: formatStudentName(s), detail: groupLine(s), keys: studentNameSearchKeys(s) }));
}

/** D1: Enter here would send the announcement to whoever is already picked. */
function keepEnterFromSubmitting(e: KeyboardEvent<HTMLInputElement>): void {
  if (e.key === "Enter") e.preventDefault();
}

function resultsNote(query: string, matches: number, total: number): string {
  if (query.trim() === "") return `${total} alumnos · escribe nombre o apellido`;
  if (matches === 0) return `Ningún alumno coincide con «${query.trim()}».`;
  if (matches > MAX_RESULTS) return `${MAX_RESULTS} de ${matches}. Escribe más para acotar.`;
  return `${matches} de ${total} alumnos`;
}

/**
 * "Un alumno": a search box and up to eight results, each with its group
 * (PLAN-redesign-avisos §10, D1, D10). One pupil at a time (B5): the chosen
 * one stays pinned above the results, once, whatever is typed next.
 */
export function StudentSearch({ students, picked, onToggle, loaded }: StudentSearchProps): ReactElement {
  const inputId = useId();
  const [query, setQuery] = useState("");
  const entries = useMemo(() => toEntries(students), [students]);
  const chosen = entries.find((e) => picked.includes(e.id));
  const matches = query.trim() === "" ? [] : entries.filter((e) => e.id !== chosen?.id && matchesSearch(query, ...e.keys));

  return (
    <div className="announce-search">
      <label htmlFor={inputId} className="announce-search__label">Buscar alumno</label>
      <input
        id={inputId}
        type="search"
        className="announce-search__input"
        autoComplete="off"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        onKeyDown={keepEnterFromSubmitting}
      />
      {chosen !== undefined && <PupilRow entry={chosen} on onToggle={onToggle} />}
      <SearchResults loaded={loaded} total={entries.length} query={query} matches={matches} onToggle={onToggle} />
    </div>
  );
}

interface SearchResultsProps {
  loaded: boolean;
  total: number;
  query: string;
  matches: PupilEntry[];
  onToggle: (id: string) => void;
}

function SearchResults({ loaded, total, query, matches, onToggle }: SearchResultsProps): ReactElement {
  if (!loaded) return <p className="announce-note">Cargando alumnos…</p>;
  if (total === 0) return <p className="announce-note">No encontramos alumnos.</p>;
  return (
    <>
      <div className="announce-rows">
        {matches.slice(0, MAX_RESULTS).map((e) => <PupilRow key={e.id} entry={e} on={false} onToggle={onToggle} />)}
      </div>
      <p className="announce-note" aria-live="polite">{resultsNote(query, matches.length, total)}</p>
    </>
  );
}

interface PupilRowProps {
  entry: PupilEntry;
  on: boolean;
  onToggle: (id: string) => void;
}

function PupilRow({ entry, on, onToggle }: PupilRowProps): ReactElement {
  return <TargetRow label={entry.name} text={entry.name} detail={entry.detail} on={on} onToggle={() => onToggle(entry.id)} mark="radio" />;
}
