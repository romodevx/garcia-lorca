"use client";

import { useMemo, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import { Toast } from "../../_design/Toast";
import { UndoToast } from "../../_design/UndoToast";
import { SelectField } from "../../_design/form";
import type { Cycle } from "../../config/api";
import { currentLoad, teacherName, type LoadSlot } from "../../teachers/load";
import { EmptyHint, ErrorAlert } from "../config/parts";
import { AddSubjectPanel } from "./AddSubjectPanel";
import { ConflictNotice } from "./ConflictNotice";
import { CurrentLoadPanel } from "./CurrentLoadPanel";
import { useLoadWrites } from "./useLoadWrites";
import { useTeacherLoadData } from "./useTeacherLoadData";

export interface TeacherLoadScreenProps {
  /** The teacher's id as the URL carries it, any case. */
  teacherId: string;
  onBack: () => void;
  /** May change the load (teachers:write). Without it the page only shows it. */
  canWrite?: boolean;
}

/**
 * Carga académica: everything one teacher teaches in a cycle, and the place to
 * add or free a class. A teacher may hold any number of classes; a class
 * (group + subject) has exactly one teacher, so taking one from a colleague is
 * always an explicit "Reemplazar".
 */
export function TeacherLoadScreen({ teacherId: rawTeacherId, onBack, canWrite = true }: TeacherLoadScreenProps): ReactElement {
  // Ids come back from the API in lower case; a hand-typed URL may not.
  // Normalised once here, so every comparison below is plain ===.
  const teacherId = rawTeacherId.toLowerCase();
  const data = useTeacherLoadData(teacherId);
  const { catalogue } = data;
  const writes = useLoadWrites({
    teacherId,
    cycleId: data.cycleId,
    catalogue,
    reload: data.reloadAssignments,
    drop: data.dropAssignment,
  });
  const load = useMemo(() => currentLoad(teacherId, catalogue), [teacherId, catalogue]);
  const name = data.teacher === undefined ? "" : teacherName(data.teacher);

  return (
    <Page>
      <button
        onClick={onBack}
        className="fade-up"
        style={{ display: "inline-flex", alignItems: "center", gap: 7, minHeight: 44, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 6 }}
      >
        <Icon name="back" size={18} stroke={2.4} />
        Volver a Docentes
      </button>

      <div className="fade-up" style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 20 }}>
        <div style={{ width: 56, height: 56, flexShrink: 0, borderRadius: 18, display: "grid", placeItems: "center", background: "var(--purple)", color: "#fff" }}>
          <Icon name="grades" size={30} />
        </div>
        <div style={{ minWidth: 0 }}>
          <h1 style={{ fontSize: 28, fontWeight: 800, color: "var(--text)", overflowWrap: "anywhere" }}>
            {name === "" ? "Carga académica" : `Carga académica de ${name}`}
          </h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>Asignaturas y grupos que imparte en el ciclo</p>
        </div>
      </div>

      {data.cycles.length > 1 && (data.status === "ready" || data.status === "error") && (
        <div style={{ maxWidth: 760, marginBottom: 18 }}>
          <CyclePicker cycles={data.cycles} cycleId={data.cycleId} onChange={data.selectCycle} />
        </div>
      )}
      {data.status === "loading" && <EmptyHint>Cargando la carga académica…</EmptyHint>}
      {data.status === "error" && (
        <div style={{ display: "grid", gap: 10, justifyItems: "start", maxWidth: 760 }}>
          <ErrorAlert message="No pudimos cargar el catálogo académico. Revisa tu permiso de Configuración o intenta de nuevo." />
          <Btn type="button" kind="forest" onClick={data.retry}>Reintentar</Btn>
        </div>
      )}
      {data.status === "ready" && data.teacher === undefined && <ErrorAlert message="No encontramos a este docente. Vuelve a Docentes y elígelo de la lista." />}
      {data.status === "ready" && data.teacher !== undefined && data.cycles.length === 0 && (
        <EmptyHint>Crea un ciclo escolar en Configuración para poder asignar materias.</EmptyHint>
      )}

      {data.status === "ready" && data.teacher !== undefined && data.cycles.length > 0 && (
        <div style={{ display: "grid", gap: 18, maxWidth: 760 }}>
          {writes.error !== undefined && <ErrorAlert message={writes.error} />}
          <CurrentLoadPanel load={load} onRemove={canWrite ? writes.remove : undefined} removing={writes.removing} />
          {canWrite && (
            <div>
              <AddSubjectPanel teacherId={teacherId} catalogue={catalogue} onAssign={writes.assign} />
              {writes.conflicts.length > 0 && (
                <ConflictNotice conflicts={writes.conflicts} onReplace={writes.replaceConflicts} onKeep={writes.keepConflicts} />
              )}
            </div>
          )}
        </div>
      )}

      {writes.undoable.length > 0 ? (
        <UndoToast message={removedMessage(writes.undoable)} onUndo={() => void writes.undo()} />
      ) : (
        writes.toast !== undefined && <Toast>{writes.toast}</Toast>
      )}
    </Page>
  );
}

function CyclePicker({ cycles, cycleId, onChange }: { cycles: readonly Cycle[]; cycleId: string; onChange: (id: string) => void }): ReactElement {
  return (
    <SelectField
      label="Ciclo"
      value={cycleId}
      options={cycles.map((c) => ({ value: c.id, label: c.status === "active" ? `${c.name} (activo)` : c.name }))}
      onChange={onChange}
      help="La carga se arma por ciclo escolar. Empieza en el activo."
    />
  );
}

function removedMessage(slots: readonly LoadSlot[]): string {
  const [only] = slots;
  if (slots.length === 1 && only !== undefined) return `Quitaste ${only.subjectName} de ${only.groupLabel}`;
  return `Quitaste ${slots.length} clases`;
}
