"use client";

import { useEffect, useId, useState, type CSSProperties, type ReactElement } from "react";

import { Icon } from "../../_design/Icon";
import { ConfigError, type Level } from "../../config/api";
import { ErrorAlert } from "./parts";

export interface LevelOrderProps {
  /** In the server's order (sortOrder): never re-sorted here. */
  levels: readonly Level[];
  /** Sends the WHOLE new order, first level first. Rejects when the move did not stick. */
  onReorder: (levelIds: string[]) => Promise<void>;
}

type Direction = "up" | "down";

/** The ids with the level at `index` swapped with its neighbour above or below. */
function moved(levels: readonly Level[], index: number, direction: Direction): string[] {
  const other = direction === "up" ? index - 1 : index + 1;
  return levels.map((l, i) => {
    if (i === index) return levels[other]?.id ?? l.id;
    if (i === other) return levels[index]?.id ?? l.id;
    return l.id;
  });
}

function refusalMessage(cause: unknown): string {
  if (cause instanceof ConfigError && cause.detail !== "" && (cause.isInvalid || cause.isConflict)) return cause.detail;
  return "No se pudo cambiar el orden de los niveles. Intenta de nuevo.";
}

/**
 * The school's levels in the order every list reads them (maternal before
 * kínder before primaria), with Subir / Bajar beside each. Buttons, not drag
 * and drop: they work from the keyboard and on a phone. The first level has no
 * Subir and the last no Bajar — absent, since a disabled arrow only says "no".
 */
export function LevelOrder({ levels, onReorder }: LevelOrderProps): ReactElement {
  const [error, setError] = useState<string | undefined>(undefined);
  const titleId = useId();
  // The arrow to focus once the list has re-rendered: the moved level's same
  // arrow, or its other one when it reached an edge and the first went away.
  const [focusAfterMove, setFocusAfterMove] = useState<{ levelId: string; direction: Direction } | undefined>(undefined);

  useEffect(() => {
    if (focusAfterMove === undefined) return;
    const { levelId, direction } = focusAfterMove;
    const other: Direction = direction === "up" ? "down" : "up";
    const find = (d: Direction) => document.querySelector<HTMLButtonElement>(`[data-level-move="${levelId}:${d}"]`);
    (find(direction) ?? find(other))?.focus();
  }, [focusAfterMove, levels]);

  const move = async (level: Level, index: number, direction: Direction): Promise<void> => {
    setError(undefined);
    setFocusAfterMove({ levelId: level.id, direction });
    try {
      await onReorder(moved(levels, index, direction));
    } catch (cause) {
      setError(refusalMessage(cause));
    }
  };

  return (
    <div style={{ marginBottom: 16 }}>
      <p id={titleId} style={{ fontSize: 13, fontWeight: 600, color: "var(--text-muted)", margin: "0 0 6px" }}>
        Orden de los niveles
      </p>
      <ol aria-labelledby={titleId} style={listStyle}>
        {levels.map((l, i) => (
          <li key={l.id} style={rowStyle}>
            <span style={{ flex: 1, fontWeight: 600, fontSize: 14.5, color: "var(--text)" }}>
              {i + 1}. {l.name}
            </span>
            {i > 0 && <MoveButton level={l} direction="up" onClick={() => void move(l, i, "up")} />}
            {i < levels.length - 1 && <MoveButton level={l} direction="down" onClick={() => void move(l, i, "down")} />}
          </li>
        ))}
      </ol>
      {error !== undefined && <ErrorAlert message={error} />}
    </div>
  );
}

function MoveButton({ level, direction, onClick }: { level: Level; direction: Direction; onClick: () => void }): ReactElement {
  const word = direction === "up" ? "Subir" : "Bajar";
  return (
    <button type="button" aria-label={`${word} ${level.name}`} data-level-move={`${level.id}:${direction}`} onClick={onClick} style={moveStyle}>
      <span aria-hidden="true" style={{ display: "inline-flex", transform: direction === "up" ? "rotate(-90deg)" : "rotate(90deg)" }}>
        <Icon name="chevron" size={16} />
      </span>
      {word}
    </button>
  );
}

const listStyle: CSSProperties = { listStyle: "none", padding: 0, margin: 0, border: "1px solid var(--border)", borderRadius: "var(--r-md)", overflow: "hidden" };

const rowStyle: CSSProperties = { display: "flex", alignItems: "center", gap: 4, minHeight: 48, padding: "2px 6px 2px 14px", borderTop: "1px solid var(--border)" };

const moveStyle: CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  gap: 4,
  minWidth: 44,
  minHeight: 44,
  padding: "0 10px",
  borderRadius: "var(--r-pill)",
  background: "transparent",
  color: "var(--text-muted)",
  fontWeight: 600,
  fontSize: 13.5,
};
