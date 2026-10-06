"use client";

import type { ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { csvName, downloadCsv } from "../../_lib/csv";
import type { StudentImportResult } from "../../studentImport/api";
import { migratedText } from "../../studentImport/copy";
import { destinationNames, reportCsv } from "../../studentImport/view";
import { ImportSummary } from "./ImportSummary";
import { alertStyle, noteStyle } from "./parts";

export interface ResultStepProps {
  result: StudentImportResult;
  /** The cut-commit sentence, when students did not answer for every row. */
  cut: string | undefined;
  onAnother: () => void;
  onViewStudents: (levelId: string | undefined) => void;
}

const CONTACTS_REMINDER =
  "Recuerda capturar los contactos de cada alumno (mamá, papá o tutor) desde su ficha: el archivo del sistema anterior no los trae.";

/** Step 3: what was written, the report with every row, and what to do next. */
export function ResultStep({ result, cut, onAnother, onViewStudents }: ResultStepProps): ReactElement {
  const names = destinationNames(result);
  const download = (): void => downloadCsv(csvName(`migracion-${names.join("-")}`, new Date()), reportCsv(result.rows));

  return (
    <div style={{ display: "grid", gap: 16 }}>
      {cut !== undefined ? (
        <p role="alert" style={alertStyle}>{cut}</p>
      ) : (
        <p role="status" style={{ fontSize: 17, fontWeight: 700, color: "var(--ok-fg)", margin: 0 }}>{migratedText(names)}</p>
      )}
      <ImportSummary summary={result.summary} onPick={() => undefined} />
      <p style={noteStyle}>{CONTACTS_REMINDER}</p>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
        <Btn icon="upload" onClick={onAnother}>Subir otro archivo</Btn>
        <Btn kind="ghost" icon="students" onClick={() => onViewStudents(result.levels[0]?.levelId)}>Ver alumnos importados</Btn>
        <Btn kind="ghost" icon="download" onClick={download}>Descargar reporte CSV</Btn>
      </div>
    </div>
  );
}
