"use client";

import { useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import type { Level } from "../../config/api";
import { ErrorAlert, SelectChip } from "./parts";

export interface CopyMonthsInlineProps {
  /** The levels that can receive the layout (the current one excluded). */
  targets: readonly Level[];
  onCopy: (toLevelIds: string[]) => Promise<void>;
}

const NO_TARGET = "Elige al menos un nivel para copiar.";
const COPY_FAILED = "No se pudieron copiar los meses. Intenta de nuevo.";

/**
 * "Copiar a otros niveles" as an inline disclosure: one choice (the levels),
 * so no dialog. The button is never disabled — copying to none says so.
 */
export function CopyMonthsInline({ targets, onCopy }: CopyMonthsInlineProps): ReactElement {
  const [open, setOpen] = useState(false);
  const [chosen, setChosen] = useState<ReadonlySet<string>>(new Set());
  const [error, setError] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);

  const toggle = (id: string): void => {
    setError(undefined);
    setChosen((prev) => {
      const next = new Set(prev);
      if (!next.delete(id)) next.add(id);
      return next;
    });
  };

  const copy = async (): Promise<void> => {
    if (busy) return;
    const ids = targets.filter((l) => chosen.has(l.id)).map((l) => l.id);
    if (ids.length === 0) {
      setError(NO_TARGET);
      return;
    }
    setBusy(true);
    try {
      await onCopy(ids);
      setOpen(false);
      setChosen(new Set());
    } catch {
      setError(COPY_FAILED);
    } finally {
      setBusy(false);
    }
  };

  if (!open) {
    return <Btn kind="ghost" icon="more" onClick={() => setOpen(true)} style={{ height: 44 }}>Copiar a otros niveles</Btn>;
  }
  return (
    <div style={{ display: "grid", gap: 10, padding: 14, borderRadius: "var(--r-md)", background: "var(--surface-2)" }}>
      <div role="group" aria-label="Niveles destino" style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
        {targets.map((l) => <SelectChip key={l.id} selected={chosen.has(l.id)} onClick={() => toggle(l.id)}>{l.name}</SelectChip>)}
      </div>
      <div style={{ display: "flex", gap: 10, alignItems: "center" }}>
        <Btn icon="check" onClick={() => void copy()} style={{ height: 44 }}>{busy ? "Copiando…" : "Copiar meses"}</Btn>
        <button type="button" onClick={() => setOpen(false)} style={{ minHeight: 44, color: "var(--text-muted)", fontWeight: 600, cursor: "pointer" }}>Cancelar</button>
      </div>
      {error !== undefined && <ErrorAlert message={error} />}
    </div>
  );
}
