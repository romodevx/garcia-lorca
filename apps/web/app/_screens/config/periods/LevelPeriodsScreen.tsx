"use client";

import { useEffect, useState, type ReactElement } from "react";

import { Btn } from "../../../_design/Controls";
import { Icon } from "../../../_design/Icon";
import { Page } from "../../../_design/Page";
import { Toast } from "../../../_design/Toast";
import { UndoToast } from "../../../_design/UndoToast";
import type { PeriodField } from "../../../config/evaluationPeriods";
import { EmptyHint, ErrorAlert, SelectChip } from "../parts";
import { GradePeriodsCard, periodFieldSelector } from "./GradePeriodsCard";
import { useLevelPeriods } from "./useLevelPeriods";

export interface LevelPeriodsScreenProps {
  levelId: string;
  /** config:write — without it the periods are shown, not edited. UX only. */
  canWrite: boolean;
  onBack: () => void;
}

/**
 * "Periodos de evaluación" of one level: every grade of the level with its own
 * periods and dates, one cycle at a time (T14). A grade's set is a dozen
 * fields or more, so it gets a page, never a drawer. Copying one grade's
 * periods to others — or to the whole level — is one click, with Deshacer.
 * The gateway (module_config + config:read/write) is the boundary.
 */
export function LevelPeriodsScreen({ levelId, canWrite, onBack }: LevelPeriodsScreenProps): ReactElement {
  const p = useLevelPeriods(levelId);
  const [focusTarget, setFocusTarget] = useState<{ key: string; field: PeriodField } | undefined>(undefined);

  // Focus moves after React has drawn the row it points at (a new row, or the
  // first field a save refused).
  useEffect(() => {
    if (focusTarget === undefined) return;
    document.querySelector<HTMLInputElement>(periodFieldSelector(focusTarget.key, focusTarget.field))?.focus();
    setFocusTarget(undefined);
  }, [focusTarget]);

  const title = p.level === undefined ? "Periodos de evaluación" : `Periodos de evaluación · ${p.level.name}`;

  return (
    <Page>
      <button onClick={onBack} style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14 }}>
        <Icon name="back" size={18} stroke={2.4} />
        Volver a Configuración
      </button>
      <h1 style={{ fontSize: 28, fontWeight: 800, color: "var(--text)", overflowWrap: "anywhere" }}>{title}</h1>
      <p style={{ color: "var(--text-muted)", fontSize: 15, margin: "4px 0 18px" }}>
        Cada grado con sus periodos y fechas. Las calificaciones se capturan por periodo.
      </p>

      {p.cycles.length > 0 && (
        <div role="group" aria-label="Ciclo" style={{ display: "flex", flexWrap: "wrap", gap: 8, marginBottom: 18 }}>
          {p.cycles.map((c) => (
            <SelectChip key={c.id} selected={c.id === p.cycle?.id} onClick={() => p.selectCycle(c.id)}>{c.name}</SelectChip>
          ))}
        </div>
      )}

      {p.load.kind === "loading" && <EmptyHint>Cargando periodos…</EmptyHint>}
      {p.load.kind === "failed" && (
        <div style={{ display: "grid", gap: 10, justifyItems: "start" }}>
          <ErrorAlert message="No se pudieron cargar los periodos de este nivel." />
          <Btn kind="ghost" onClick={p.retry}>Reintentar</Btn>
        </div>
      )}
      {p.load.kind === "ready" && p.cycle === undefined && <EmptyHint>Primero crea un ciclo escolar en Configuración.</EmptyHint>}
      {p.load.kind === "ready" && p.cycle !== undefined && p.grades.length === 0 && (
        <EmptyHint>Este nivel aún no tiene grados.</EmptyHint>
      )}

      {p.load.kind === "ready" && p.cycle !== undefined && (
        <div style={{ display: "grid", gap: 18 }}>
          {p.grades.map((g) => (
            <GradePeriodsCard
              key={`${p.cycle?.id ?? ""}:${g.id}`}
              grade={g}
              others={p.grades.filter((o) => o.id !== g.id)}
              cycle={p.cycle as NonNullable<typeof p.cycle>}
              rows={p.drafts[g.id] ?? []}
              problems={p.problems[g.id] ?? []}
              error={p.errors[g.id]}
              busy={p.busy[g.id] === true}
              canWrite={canWrite}
              onEdit={(key, field, value) => p.edit(g.id, key, field, value)}
              onBlur={(key, field) => p.blur(g.id, key, field)}
              onAdd={() => setFocusTarget({ key: p.add(g.id), field: "name" })}
              onRemove={(key) => p.remove(g.id, key)}
              onSave={() => {
                void p.save(g.id).then((first) => first !== undefined && setFocusTarget({ key: first.key, field: first.field }));
              }}
              onCopy={async (ids) => {
                const outcome = await p.copy(g.id, ids);
                if (outcome.focus !== undefined) setFocusTarget({ key: outcome.focus.key, field: outcome.focus.field });
                return outcome.copied;
              }}
            />
          ))}
        </div>
      )}

      {p.undo !== undefined && <UndoToast message={p.undo.message} onUndo={() => void p.takeBack()} onDismiss={p.dismissUndo} />}
      {p.undo === undefined && p.notice !== undefined && <Toast>{p.notice}</Toast>}
    </Page>
  );
}
