"use client";

import { useEffect, useMemo, useState, type ReactElement } from "react";

import { ConfirmDialog } from "../../_design/ConfirmDialog";
import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import { Toast } from "../../_design/Toast";
import { UndoToast } from "../../_design/UndoToast";
import { DualPanelPicker } from "../../_design/students/DualPanelPicker";
import { useStudentRoster, type RosterStudent } from "../../_design/students/useStudentRoster";
import { API_URL } from "../../api-url";
import { assignScholarship, listScholarshipHolders, unassignScholarship, type Scholarship, type ScholarshipOverlap, type ScholarshipTarget } from "../../scholarships/api";
import { overlapWarning, scholarshipTitle, scholarshipValueLabel, targetsSummary } from "../../scholarships/format";
import { LONG_TOAST_MS, useToast, useUndoOffer } from "../activities/useToast";
import { ErrorAlert } from "../config/parts";

const LABELS = { available: "Alumnos", assigned: "Con beca", assign: "Asignar beca", unassign: "Quitar beca", emptyAssigned: "Nadie tiene esta beca todavía." };

const plural = (n: number, one: string, many: string): string => `${n} ${n === 1 ? one : many}`;

export interface ScholarshipAssignPageProps {
  scholarship: Scholarship;
  targets: readonly ScholarshipTarget[];
  canWrite: boolean;
  onBack: () => void;
}

/**
 * Who holds a scholarship (T18): the shared DualPanelPicker, "Con beca" on the
 * right with a warning beside whoever also holds another one on the same
 * charges (they do not stack). Giving it changes pending charges at once, so
 * moving pupils in asks first; taking it away does not — it is done at once
 * and offers Deshacer, which does not time out (CC-210).
 */
export function ScholarshipAssignPage({ scholarship, targets, canWrite, onBack }: ScholarshipAssignPageProps): ReactElement {
  const roster = useStudentRoster();
  const title = scholarshipTitle(scholarship.name);
  const writable = canWrite && scholarship.active;
  /** studentId → the other scholarships they hold on the same charges. */
  const [holders, setHolders] = useState<ReadonlyMap<string, ScholarshipOverlap[]>>(new Map());
  const holderIds = useMemo(() => [...holders.keys()], [holders]);
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const { toast, say } = useToast();
  const { offer, offerUndo, withdraw } = useUndoOffer();
  const [asking, setAsking] = useState<{ ids: string[]; answer: (yes: boolean) => void } | null>(null);

  const confirmAssign = (ids: string[]): Promise<boolean> => new Promise((answer) => setAsking({ ids, answer }));
  const reply = (yes: boolean): void => {
    asking?.answer(yes);
    setAsking(null);
  };

  useEffect(() => {
    let live = true;
    listScholarshipHolders(API_URL, scholarship.id)
      .then((list) => live && setHolders(new Map(list.map((h) => [h.studentId, h.overlaps]))))
      .catch((err: unknown) => live && setFailure(err instanceof Error ? err.message : "No se pudo leer quién tiene la beca."));
    return () => {
      live = false;
    };
  }, [scholarship.id]);

  const run = async (work: () => Promise<void>): Promise<void> => {
    if (busy) return;
    setBusy(true);
    setFailure(undefined);
    withdraw();
    try {
      await work();
    } catch (err: unknown) {
      setFailure(err instanceof Error ? err.message : "No se pudo guardar.");
    } finally {
      setBusy(false);
    }
  };

  const assign = (ids: string[]): void => void run(async () => {
    const added = await assignScholarship(API_URL, scholarship.id, ids);
    setHolders((prev) => new Map([...prev, ...added.map((h): [string, ScholarshipOverlap[]] => [h.studentId, h.overlaps])]));
    say(`${plural(added.length, "alumno", "alumnos")} con ${title}`, LONG_TOAST_MS);
  });

  // Giving it back is the way back (CC-210): it re-applies to the pending
  // charges exactly as the first assignment did. No second confirmation — the
  // person is undoing, not deciding something new.
  const reassign = (ids: string[]): void => void run(async () => {
    const added = await assignScholarship(API_URL, scholarship.id, ids);
    setHolders((prev) => new Map([...prev, ...added.map((h): [string, ScholarshipOverlap[]] => [h.studentId, h.overlaps])]));
    say(`Deshecho: ${plural(added.length, "alumno", "alumnos")} con ${title} otra vez`, LONG_TOAST_MS);
  });

  const unassign = (ids: string[]): void => void run(async () => {
    const removed = await unassignScholarship(API_URL, scholarship.id, ids);
    setHolders((prev) => new Map([...prev].filter(([id]) => !ids.includes(id))));
    offerUndo({ message: `${plural(removed, "alumno", "alumnos")} ya sin ${title}`, restore: () => reassign(ids) });
  });

  const overlapBadge = (s: RosterStudent): ReactElement | null => {
    const overlaps = holders.get(s.id) ?? [];
    if (overlaps.length === 0) return null;
    return (
      <span style={{ display: "inline-flex", alignItems: "center", gap: 6, maxWidth: 280, padding: "4px 10px", borderRadius: "var(--r-md)", fontSize: 12.5, fontWeight: 600, background: "var(--warn-bg)", color: "var(--warn-fg)" }}>
        <Icon name="warning" size={14} stroke={2.4} />
        {overlapWarning(overlaps)}
      </span>
    );
  };

  return (
    <Page>
      <button onClick={onBack} style={{ display: "inline-flex", alignItems: "center", gap: 7, minHeight: 44, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 8 }}>
        <Icon name="back" size={18} stroke={2.4} />Volver a becas
      </button>
      <header style={{ marginBottom: 18 }}>
        <h1 style={{ fontSize: 28, fontWeight: 800, color: "var(--text)" }}>{`Alumnos con ${title}`}</h1>
        <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>{`${scholarshipValueLabel(scholarship.kind, scholarship.value)} · ${targetsSummary(scholarship.targets, targets)}`}</p>
      </header>

      {canWrite && !scholarship.active && (
        <p role="status" style={{ margin: "0 0 14px", fontSize: 14.5, color: "var(--warn-fg)" }}>Esta beca está inactiva: actívala en Editar para poder asignarla.</p>
      )}
      {failure !== undefined && <ErrorAlert message={failure} />}
      {roster.status === "failed" && <ErrorAlert message="No se pudo leer la lista de alumnos." />}
      {roster.status === "loading" && <p style={{ color: "var(--text-muted)" }}>Cargando alumnos…</p>}
      {roster.status === "ok" && (
        <DualPanelPicker
          students={roster.students}
          catalog={roster.catalog}
          assignedIds={holderIds}
          onAssign={assign}
          onUnassign={unassign}
          labels={LABELS}
          describe={(s) => s.room}
          readOnly={!writable}
          confirmAssign={confirmAssign}
          renderAssignedExtra={overlapBadge}
        />
      )}
      {asking !== null && (
        <ConfirmDialog title="Asignar beca" confirmLabel="Asignar" icon="scholarship" onConfirm={() => reply(true)} onCancel={() => reply(false)}>
          <p style={{ margin: 0 }}><b>{`¿Dar ${title} a ${plural(asking.ids.length, "alumno", "alumnos")}?`}</b></p>
          <p style={{ margin: "10px 0 0", color: "var(--text-muted)" }}>
            Se aplica en el acto a sus cargos pendientes, mientras los paguen a tiempo. Lo ya pagado no cambia.
          </p>
        </ConfirmDialog>
      )}
      {offer !== undefined
        ? <UndoToast message={offer.message} onUndo={offer.restore} />
        : toast !== undefined && <Toast>{toast}</Toast>}
    </Page>
  );
}
