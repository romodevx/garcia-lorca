"use client";

import { useId, useState, type ReactElement } from "react";

import { inputStyle } from "../../_design/form";
import { Icon } from "../../_design/Icon";
import { matchesSearch } from "../../_design/textSearch";
import type { FamilyPupil } from "./familyPupil";

/** A search shows a handful; more than this means the office should type more. */
const MAX_MATCHES = 8;

const LABEL = { fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)" } as const;

/**
 * Finds a pupil by name or classroom among `candidates` and hands it back.
 * Nothing is listed until something is typed: six hundred names are not a
 * choice, they are a scroll.
 */
export function PupilSearch({ label, candidates, onPick }: {
  label: string;
  candidates: readonly FamilyPupil[];
  onPick: (pupil: FamilyPupil) => void;
}): ReactElement {
  const id = useId();
  const [query, setQuery] = useState("");
  const matches = query.trim() === "" ? [] : candidates.filter((p) => matchesSearch(query, p.name, p.room)).slice(0, MAX_MATCHES);

  return (
    <div style={{ display: "grid", gap: 6 }}>
      <label htmlFor={id} style={LABEL}>{label}</label>
      <input
        id={id}
        type="search"
        value={query}
        autoComplete="off"
        onChange={(e) => setQuery(e.target.value)}
        // Enter picks the only match instead of submitting the drawer.
        onKeyDown={(e) => {
          if (e.key !== "Enter") return;
          e.preventDefault();
          const only = matches.length === 1 ? matches[0] : undefined;
          if (only !== undefined) {
            onPick(only);
            setQuery("");
          }
        }}
        style={{ ...inputStyle(false), fontSize: 16 }}
      />
      {query.trim() !== "" && matches.length === 0 && (
        <p style={{ fontSize: 13.5, color: "var(--text-faint)", margin: 0 }}>Ningún alumno sin familia con ese nombre.</p>
      )}
      {matches.length > 0 && (
        <ul style={{ display: "grid", gap: 4, listStyle: "none", margin: 0, padding: 0 }}>
          {matches.map((p) => (
            <li key={p.id}>
              <button
                type="button"
                aria-label={`Agregar a ${p.name}`}
                onClick={() => {
                  onPick(p);
                  setQuery("");
                }}
                style={{ width: "100%", minHeight: 44, display: "flex", alignItems: "center", gap: 10, padding: "6px 12px", borderRadius: "var(--r-md)", background: "var(--surface-2)", border: "1px solid var(--border)", textAlign: "left", cursor: "pointer" }}
              >
                <Icon name="plus" size={16} />
                <span style={{ flex: 1, minWidth: 0, fontSize: 14.5, fontWeight: 600, color: "var(--text)" }}>{p.name}</span>
                <span style={{ fontSize: 12.5, color: "var(--text-faint)" }}>{p.room}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
