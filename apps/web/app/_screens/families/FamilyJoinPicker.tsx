"use client";

import { useEffect, useId, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { inputStyle } from "../../_design/form";
import { matchesSearch } from "../../_design/textSearch";
import { API_URL } from "../../api-url";
import { addStudentsToFamily, createFamily, familyRefusal, listFamilies, type Family } from "../../families/api";

const MAX_MATCHES = 6;
const LABEL = { fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)" } as const;

/**
 * "Agregar a una familia" from a pupil's file: an existing family found by
 * name, or a new one with this pupil (named by the server after its surnames).
 */
export function FamilyJoinPicker({ studentId, onJoined }: { studentId: string; onJoined: (familyId: string) => void }): ReactElement {
  const id = useId();
  const [families, setFamilies] = useState<readonly Family[]>([]);
  const [query, setQuery] = useState("");
  const [error, setError] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let alive = true;
    listFamilies(API_URL).then((rows) => { if (alive) setFamilies(rows); }).catch(() => { if (alive) setError("No pudimos leer las familias."); });
    return () => { alive = false; };
  }, []);

  const join = async (write: () => Promise<Family>): Promise<void> => {
    if (busy) return;
    setBusy(true);
    setError(undefined);
    try {
      onJoined((await write()).id);
    } catch (cause) {
      setError(familyRefusal(cause, "No se pudo agregar a la familia."));
      setBusy(false);
    }
  };
  const matches = query.trim() === "" ? [] : families.filter((f) => matchesSearch(query, f.name)).slice(0, MAX_MATCHES);

  return (
    <div style={{ display: "grid", gap: 10 }}>
      <label htmlFor={id} style={LABEL}>Buscar familia</label>
      <input id={id} type="search" value={query} autoComplete="off" onChange={(e) => setQuery(e.target.value)} style={{ ...inputStyle(false), fontSize: 16 }} />
      {matches.map((f) => (
        <Btn key={f.id} kind="ghost" busy={busy} onClick={() => void join(() => addStudentsToFamily(API_URL, f.id, [studentId]))} aria-label={`Agregar a la familia ${f.name}`} style={{ justifyContent: "flex-start", height: 44, fontSize: 14.5 }}>
          {f.name}
        </Btn>
      ))}
      <div>
        <Btn kind="ghost" icon="plus" busy={busy} onClick={() => void join(() => createFamily(API_URL, { name: "", studentIds: [studentId] }))} style={{ height: 44, fontSize: 14.5 }}>
          Crear una familia nueva con este alumno
        </Btn>
      </div>
      {error !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 13.5, margin: 0 }}>{error}</p>}
    </div>
  );
}
