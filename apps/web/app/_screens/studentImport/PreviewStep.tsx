"use client";

import { useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import type { Level } from "../../config/api";
import type { LevelChoice, StudentImportResult } from "../../studentImport/api";
import { PREVIEW_BANNER, allAlreadyText, confirmationText, importButtonLabel, moveButtonLabel, toMoveText } from "../../studentImport/copy";
import { defaultTab, destinationNames, type ImportTab } from "../../studentImport/view";
import { CatalogTree } from "./CatalogTree";
import { ImportSummary } from "./ImportSummary";
import { ImportTable } from "./ImportTable";
import { LevelMapping } from "./LevelMapping";
import { noteStyle } from "./parts";

export interface PreviewStepProps {
  result: StudentImportResult;
  levels: readonly Level[];
  onChoose: (fileName: string, choice: LevelChoice) => void;
  onCapacity: (key: string, capacity: number) => void;
  onImport: () => void;
  onBack: () => void;
  onCancel: () => void;
  onOpenTuition: () => void;
  /** Bumped by the screen when the server says levels are unresolved (422). */
  focusUnresolved: number;
}

/** Step 2: what the import would do. Nothing is saved until the button is pressed. */
export function PreviewStep(props: PreviewStepProps): ReactElement {
  const { result, levels, onChoose, onCapacity, onImport, onBack, onCancel, onOpenTuition, focusUnresolved } = props;
  const [tab, setTab] = useState<ImportTab>(() => defaultTab(result.rows));
  const [asked, setAsked] = useState(0);
  const { summary } = result;
  const newPupils = summary.new + summary.pending;
  const toMove = summary.movedToCycleStart;
  const nothingToImport = newPupils === 0 && toMove === 0;
  const buttonLabel = newPupils > 0 ? importButtonLabel(newPupils, destinationNames(result)) : moveButtonLabel(toMove);

  const tryImport = (): void => {
    if (result.unresolved.length > 0) {
      setAsked((n) => n + 1);
      return;
    }
    onImport();
  };

  return (
    <div style={{ display: "grid", gap: 16 }}>
      <p role="note" style={{ ...noteStyle, fontWeight: 700 }}>{PREVIEW_BANNER}</p>
      <ImportSummary summary={summary} onPick={setTab} />
      <LevelMapping fileLevels={result.levels} levels={levels} onChoose={onChoose} onOpenTuition={onOpenTuition} focusRequest={focusUnresolved + asked} />
      <CatalogTree placements={result.placements} onCapacity={onCapacity} />
      <ImportTable rows={result.rows} tab={tab} onTab={setTab} />

      {nothingToImport ? (
        <p role="status" style={{ ...noteStyle, fontWeight: 600 }}>
          {summary.already > 0 ? allAlreadyText(summary.already) : "Ninguna fila de este archivo se puede importar. Revisa los motivos en la tabla."}
        </p>
      ) : (
        newPupils > 0 && <p style={noteStyle}>{confirmationText(result.cycle.name)}</p>
      )}
      {toMove > 0 && <p style={noteStyle}>{toMoveText(toMove)}</p>}
      <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 12 }}>
        {!nothingToImport && <Btn icon="upload" onClick={tryImport}>{buttonLabel}</Btn>}
        <Btn kind="ghost" onClick={onBack}>Atrás</Btn>
        <button type="button" onClick={onCancel} style={{ minHeight: 44, padding: "0 12px", color: "var(--text-muted)", fontWeight: 600 }}>Cancelar</button>
      </div>
    </div>
  );
}
