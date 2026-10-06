"use client";

import { useEffect, useRef, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import type { Level } from "../../config/api";
import type { ImportLevel, LevelChoice } from "../../studentImport/api";
import { pupils } from "../../studentImport/copy";
import { Chip, cardStyle, warnStyle } from "./parts";

export interface LevelMappingProps {
  fileLevels: readonly ImportLevel[];
  /** The school's levels: the chips an office picks from. */
  levels: readonly Level[];
  onChoose: (fileName: string, choice: LevelChoice) => void;
  onOpenTuition: () => void;
  /** Bumped to send focus to the first card still without a destination. */
  focusRequest: number;
}

/** One card per "Escolaridad" of the file. An unmatched one has NO default (decision 15). */
export function LevelMapping({ fileLevels, levels, onChoose, onOpenTuition, focusRequest }: LevelMappingProps): ReactElement {
  const box = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    if (focusRequest === 0) return;
    box.current?.querySelector<HTMLElement>('[data-unresolved="true"]')?.focus();
  }, [focusRequest]);

  return (
    <div ref={box} style={{ display: "grid", gap: 12 }}>
      {fileLevels.map((fl) => (
        <LevelCard key={fl.fileName} fileLevel={fl} levels={levels} onChoose={onChoose} onOpenTuition={onOpenTuition} />
      ))}
    </div>
  );
}

interface LevelCardProps {
  fileLevel: ImportLevel;
  levels: readonly Level[];
  onChoose: (fileName: string, choice: LevelChoice) => void;
  onOpenTuition: () => void;
}

function LevelCard({ fileLevel, levels, onChoose, onOpenTuition }: LevelCardProps): ReactElement {
  const unresolved = fileLevel.status === "unresolved";
  const [changing, setChanging] = useState(false);
  const title = unresolved ? `Elige a dónde van los alumnos de «${fileLevel.fileName}»` : `«${fileLevel.fileName}» del archivo`;
  const choose = (choice: LevelChoice): void => {
    setChanging(false);
    onChoose(fileLevel.fileName, choice);
  };
  const destination = fileLevel.status === "create" ? `Se creará el nivel «${fileLevel.fileName}»` : `Van a ${fileLevel.levelName ?? fileLevel.fileName}`;

  return (
    <section
      aria-label={title}
      tabIndex={-1}
      data-unresolved={unresolved ? "true" : undefined}
      style={{ ...cardStyle, border: unresolved ? "1.5px solid var(--warn-fg)" : cardStyle.border }}
    >
      <h3 style={{ fontSize: 16.5, fontWeight: 700, color: "var(--text)", overflowWrap: "anywhere" }}>{title}</h3>
      <p style={{ fontSize: 14, color: "var(--text-muted)", margin: 0 }}>{pupils(fileLevel.students)} en el archivo</p>
      {!unresolved && (
        <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 10 }}>
          <p style={{ fontSize: 15, fontWeight: 600, color: "var(--text)", margin: 0 }}>{destination}</p>
          {!changing && <Btn kind="ghost" onClick={() => setChanging(true)} style={{ height: 44 }}>Cambiar</Btn>}
        </div>
      )}
      {(unresolved || changing) && (
        <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
          {levels.map((l) => (
            <Chip key={l.id} selected={fileLevel.levelId === l.id} onClick={() => choose({ levelId: l.id })}>{l.name}</Chip>
          ))}
          <Chip selected={fileLevel.status === "create"} onClick={() => choose({ create: true })}>{`+ Crear nivel nuevo «${fileLevel.fileName}»`}</Chip>
        </div>
      )}
      {fileLevel.noTuition && (
        <div style={{ ...warnStyle, display: "flex", flexWrap: "wrap", alignItems: "center", gap: 10 }}>
          <span>Sin tabulador en el ciclo activo: no se cobrará colegiatura hasta que lo captures</span>
          <Btn kind="ghost" onClick={onOpenTuition} style={{ height: 44 }}>Ir a Colegiaturas</Btn>
        </div>
      )}
    </section>
  );
}
