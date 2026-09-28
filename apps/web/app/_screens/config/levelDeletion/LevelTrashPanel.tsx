"use client";

import type { ReactElement } from "react";

import { Btn } from "../../../_design/Controls";
import { DrawerShell } from "../../../_design/DrawerShell";
import { Toast } from "../../../_design/Toast";
import type { LevelTrashItem } from "../../../config/api";
import { EmptyHint, ErrorAlert, Section } from "../parts";
import { daysLeft, longDay, restoredSummary, skippedLine, trashSummary } from "./copy";
import { useLevelTrash, type LevelTrash, type RestoreOutcome } from "./useLevelTrash";

export interface LevelTrashPanelProps {
  /** The levels list changed: a restored level is back. */
  onRestored: () => void;
  /** Injectable clock, so "Quedan N días" is deterministic under test. */
  now?: () => Date;
}

/**
 * Deleted levels, restorable for 7 days (owner-only, like the delete). The
 * restore asks once: it is reversible — the level can be deleted again — but
 * it brings back a whole level into the catalogue.
 */
export function LevelTrashPanel({ onRestored, now = () => new Date() }: LevelTrashPanelProps): ReactElement {
  const trash = useLevelTrash(onRestored);
  return (
    <Section title="Papelera" subtitle="Niveles eliminados. Se pueden restaurar durante 7 días.">
      {trash.outcome !== undefined && <RestoreReport outcome={trash.outcome} />}
      {trash.load.kind === "loading" && <EmptyHint>Cargando papelera…</EmptyHint>}
      {trash.load.kind === "failed" && <ErrorAlert message="No se pudo cargar la papelera." />}
      {trash.load.kind === "ready" && trash.load.items.length === 0 && <EmptyHint>La papelera está vacía</EmptyHint>}
      {trash.load.kind === "ready" && trash.load.items.length > 0 && (
        <ul style={{ listStyle: "none", padding: 0, margin: 0, display: "grid", gap: 10 }}>
          {trash.load.items.map((item) => <TrashRow key={item.batchId} item={item} left={daysLeft(item.restorableUntil, now())} onRestore={trash.ask} />)}
        </ul>
      )}
      {trash.confirming !== undefined && <RestoreDialog trash={trash} item={trash.confirming} />}
    </Section>
  );
}

function TrashRow({ item, left, onRestore }: { item: LevelTrashItem; left: number; onRestore: (item: LevelTrashItem) => void }): ReactElement {
  return (
    <li style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 12, padding: "14px 16px", borderRadius: "var(--r-lg)", border: "1px solid var(--border)", background: "var(--surface-2)" }}>
      <div style={{ flex: "1 1 220px", minWidth: 0, overflowWrap: "anywhere" }}>
        <p style={{ fontWeight: 700, color: "var(--text)" }}>{item.levelName}</p>
        <p style={{ fontSize: 13.5, color: "var(--text-muted)" }}>{`Eliminado el ${longDay(item.deletedAt)} por ${item.deletedByName}`}</p>
        <p style={{ fontSize: 13.5, color: "var(--text-muted)" }}>{trashSummary(item.counts)}</p>
      </div>
      {left > 0 ? (
        <>
          <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--warn-fg)" }}>{left === 1 ? "Queda 1 día" : `Quedan ${left} días`}</span>
          <Btn kind="ghost" onClick={() => onRestore(item)} aria-label={`Restaurar ${item.levelName}`}>Restaurar</Btn>
        </>
      ) : (
        <span style={{ fontSize: 13.5, color: "var(--text-muted)" }}>Ya no se puede restaurar</span>
      )}
    </li>
  );
}

function RestoreDialog({ trash, item }: { trash: LevelTrash; item: LevelTrashItem }): ReactElement | null {
  return (
    <DrawerShell
      title={`¿Restaurar ${item.levelName}?`}
      icon="back"
      size="sm"
      onClose={trash.cancel}
      onSubmit={() => void trash.restore()}
      footer={
        <div style={{ display: "flex", flexWrap: "wrap", justifyContent: "flex-end", gap: 10 }}>
          <button type="button" onClick={trash.cancel} style={{ minHeight: 44, padding: "0 12px", color: "var(--text-muted)", fontWeight: 600 }}>Cancelar</button>
          <Btn type="submit">Restaurar nivel</Btn>
        </div>
      }
    >
      <p style={{ fontSize: 15, color: "var(--text)" }}>
        Vuelven sus grados, grupos, asignaturas, calificaciones y demás, y sus alumnos regresan a sus grupos cuando todavía es posible.
      </p>
      {trash.restoreError !== undefined && <p role="alert" style={{ marginTop: 12, color: "var(--bad-fg)", fontWeight: 600 }}>{trash.restoreError}</p>}
    </DrawerShell>
  );
}

/** The toast says it came back; the list below it stays, because a toast leaves. */
function RestoreReport({ outcome }: { outcome: RestoreOutcome }): ReactElement {
  const { skipped } = outcome.result;
  return (
    <>
      <Toast>{restoredSummary(outcome.levelName, outcome.result.restored)}</Toast>
      {skipped.length > 0 && (
        <div style={{ marginBottom: 14, padding: "12px 14px", borderRadius: "var(--r-md)", background: "var(--warn-bg)", color: "var(--warn-fg)" }}>
          <p style={{ fontWeight: 700 }}>No todo se pudo restaurar:</p>
          <ul style={{ margin: "6px 0 0", paddingLeft: 20 }}>{skipped.map((s) => <li key={s.kind}>{skippedLine(s)}</li>)}</ul>
        </div>
      )}
    </>
  );
}
