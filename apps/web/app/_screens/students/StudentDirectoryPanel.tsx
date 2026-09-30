"use client";

import { useMemo, useState, type ReactElement } from "react";
import type { AdmissionStatus, Student } from "../../students/api";
import { TextField } from "../../_design/form";
import { EmptyHint, Section } from "../config/parts";
import { matchesSearch } from "../../_design/textSearch";
import { formatStudentName, sortStudents } from "../../_design/students/studentName";

const STATUS_LABEL: Record<AdmissionStatus, string> = {
  pending: "Pendiente",
  accepted: "Aceptado",
  enrolled: "Inscrito",
  rejected: "Rechazado",
  withdrawn: "Baja",
};

const STATUS_COLOR: Record<AdmissionStatus, string> = {
  pending: "var(--text-muted)",
  accepted: "var(--forest)",
  enrolled: "var(--forest)",
  rejected: "var(--bad-fg)",
  withdrawn: "var(--bad-fg)",
};

export interface StudentDirectoryPanelProps {
  students: readonly Student[];
}

/**
 * A browsable, searchable directory of EVERY student — the school-wide list the
 * per-group roster can't give. Filters client-side by name, CURP, grade-group or
 * guardian so a director can find anyone without picking a group first. Read-only
 * (creation/enrollment/transfer live in the sibling panels); the gateway is the
 * security boundary.
 */
export function StudentDirectoryPanel({ students }: StudentDirectoryPanelProps): ReactElement {
  const [query, setQuery] = useState("");

  const rows = useMemo(() => {
    return sortStudents(students).filter((s) => matchesSearch(query, formatStudentName(s), s.curp, s.gradeGroup, s.guardianName));
  }, [students, query]);

  const th: React.CSSProperties = { textAlign: "left", padding: "8px 10px", color: "var(--text-muted)", fontSize: 12.5, fontWeight: 600, borderBottom: "1px solid var(--border)" };
  const td: React.CSSProperties = { padding: "9px 10px", fontSize: 13.5, borderBottom: "1px solid var(--surface-3)" };

  return (
    <Section title="Directorio de alumnos" subtitle={`${students.length} alumnos · busca por nombre, CURP, grupo o tutor`}>
      <div style={{ maxWidth: 420, marginBottom: 12 }}>
        <TextField label="Buscar" value={query} onChange={setQuery} help="Puedes escribir el nombre, la CURP, el grupo o el nombre del tutor." />
      </div>
      {rows.length === 0 ? (
        <EmptyHint>Sin resultados para “{query}”.</EmptyHint>
      ) : (
        <div style={{ overflowX: "auto" }}>
          <table style={{ width: "100%", borderCollapse: "collapse" }}>
            <thead>
              <tr>
                <th style={th}>Alumno</th>
                <th style={th}>Nivel</th>
                <th style={th}>Grado y grupo</th>
                <th style={th}>Tutor</th>
                <th style={th}>CURP</th>
                <th style={th}>Estado</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((s) => (
                <tr key={s.id}>
                  <td style={{ ...td, fontWeight: 600 }}>{formatStudentName(s)}</td>
                  <td style={{ ...td, textTransform: "capitalize" }}>{s.level}</td>
                  <td style={td}>{s.gradeGroup}</td>
                  <td style={{ ...td, color: "var(--text-muted)" }}>{s.guardianName}</td>
                  <td style={{ ...td, color: "var(--text-muted)", fontFamily: "monospace", fontSize: 12 }}>{s.curp}</td>
                  <td style={{ ...td, color: STATUS_COLOR[s.admissionStatus], fontWeight: 600 }}>{STATUS_LABEL[s.admissionStatus]}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Section>
  );
}
