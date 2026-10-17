"use client";

import { useEffect, useMemo, useState, type ReactElement, type Ref } from "react";
import { listStudents, type Student } from "../../students/api";
import { field } from "./parts";

import { API_URL } from "../../api-url";
import { matchesSearch } from "../../_design/textSearch";
import { formatStudentName, sortStudents, studentNameSearchKeys } from "../../_design/students/studentName";


/**
 * Picks the pupil a counter sale belongs to.
 *
 * It exists because the first version of this field was a lie: it invited a
 * cashier to type a name and then sent that name where an id belongs. The
 * service, correctly, had no idea what to do with it. A field that looks like a
 * search has to BE a search.
 *
 * The roster is loaded once and filtered in the browser. A school has a few
 * hundred pupils, so this is a small list and one request; a per-keystroke
 * request would put the network between a cashier and a queue for no gain.
 */
export function StudentPicker({
  value,
  onChange,
  inputRef,
}: {
  /** The chosen pupil, or null while nobody is chosen. */
  value: Student | null;
  onChange: (student: Student | null) => void;
  /** The search field, for a screen that sends the cursor to it. */
  inputRef?: Ref<HTMLInputElement>;
}): ReactElement {
  const [students, setStudents] = useState<Student[] | null>(null);
  const [term, setTerm] = useState("");
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let alive = true;
    listStudents(API_URL)
      .then((roster) => {
        if (alive) setStudents(roster);
      })
      .catch(() => {
        if (alive) {
          setStudents([]);
          setFailed(true);
        }
      });
    return () => {
      alive = false;
    };
  }, []);

  const matches = useMemo(() => {
    if (term.trim() === "" || students === null) return [];
    return sortStudents(students.filter((s) => matchesSearch(term, ...studentNameSearchKeys(s), s.curp))).slice(0, 6);
  }, [students, term]);

  if (value !== null) {
    return (
      <div style={{ display: "flex", alignItems: "center", gap: 12, padding: "12px 14px", borderRadius: "var(--r-md)", background: "var(--surface-3)", border: "1px solid var(--border)" }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <p style={{ fontWeight: 700, fontSize: 15 }}>{formatStudentName(value)}</p>
          <p style={{ fontSize: 13, color: "var(--text-muted)" }}>
            {value.gradeGroup !== "" ? value.gradeGroup : value.level}
          </p>
        </div>
        <button
          onClick={() => {
            onChange(null);
            setTerm("");
          }}
          style={{ fontSize: 13.5, color: "var(--text-muted)", textDecoration: "underline" }}
        >
          Cambiar
        </button>
      </div>
    );
  }

  return (
    <div>
      <input
        ref={inputRef}
        style={field}
        placeholder="Escribe el nombre del alumno"
        aria-label="Alumno"
        value={term}
        onChange={(e) => setTerm(e.target.value)}
        autoComplete="off"
      />
      {students === null && <p style={hint}>Cargando alumnos…</p>}
      {failed && (
        <p style={hint}>
          No pudimos cargar la lista de alumnos. Puedes registrar la venta sin alumno con una nota.
        </p>
      )}
      {term.trim() !== "" && students !== null && matches.length === 0 && !failed && (
        <p style={hint}>Ningún alumno coincide. Revisa el nombre o registra la venta sin alumno.</p>
      )}
      {matches.length > 0 && (
        <ul style={{ marginTop: 8, display: "grid", gap: 4 }}>
          {matches.map((student) => (
            <li key={student.id}>
              <button
                onClick={() => {
                  onChange(student);
                  setTerm("");
                }}
                style={{
                  width: "100%",
                  textAlign: "left",
                  padding: "10px 14px",
                  borderRadius: "var(--r-md)",
                  border: "1px solid var(--border)",
                  background: "var(--surface)",
                }}
              >
                <span style={{ fontWeight: 600, fontSize: 14.5 }}>{formatStudentName(student)}</span>
                <span style={{ fontSize: 13, color: "var(--text-muted)" }}>
                  {" · "}
                  {student.gradeGroup !== "" ? student.gradeGroup : student.level}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

const hint = { fontSize: 12.5, color: "var(--text-faint)", marginTop: 6 } as const;
