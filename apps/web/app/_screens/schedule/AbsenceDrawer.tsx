"use client";

import { useState, type KeyboardEvent, type ReactElement } from "react";

import { DateField, SelectField, TextArea, useRequired } from "../../_design/form";
import { SlideOver } from "../../_design/SlideOver";
import { ChipRow } from "../../_design/students/StudentFilters";
import { API_URL } from "../../api-url";
import { createAbsence, updateAbsence, type AbsenceInput, type AbsenceKind, type AbsenceSaved, type TeacherAbsence } from "../../schedule/api";
import { timeOptions, toMinutes } from "../../schedule/time";
import { chipOption } from "./parts";

/** Incapacidad, Vacaciones, Asunto personal, Otro — the order the school reads them. */
export const ABSENCE_KIND_LABEL: Record<AbsenceKind, string> = {
  sick: "Incapacidad",
  vacation: "Vacaciones",
  personal: "Asunto personal",
  other: "Otro",
};

const KINDS = (Object.keys(ABSENCE_KIND_LABEL) as AbsenceKind[]).map((k) => chipOption(k, ABSENCE_KIND_LABEL[k]));
/** An appointment fits between 06:00 and 22:00; the grid is narrower still. */
const HOURS = timeOptions(6 * 60, 22 * 60).map((t) => ({ value: t, label: t }));
const NOTE_MAX = 500;

export interface AbsenceDrawerProps {
  teachers: readonly { id: string; firstName: string; lastName: string }[];
  /** The cycle of the timetable on screen; omitted, config takes the cycle of the dates. */
  cycleId?: string;
  /** The school's date (aaaa-mm-dd): the default first day. */
  today: string;
  /** Opened from a teacher's row: that teacher, already chosen. */
  teacherId?: string;
  /** Editing an existing absence (never its teacher). */
  editing?: TeacherAbsence;
  onClose: () => void;
  onSaved: (saved: AbsenceSaved) => void;
}

interface Values {
  teacherId: string;
  kind: AbsenceKind | "";
  startsOn: string;
  endsOn: string;
  allDay: boolean;
  startsAt: string;
  endsAt: string;
  note: string;
}

function initialValues(p: AbsenceDrawerProps): Values {
  const a = p.editing;
  if (a !== undefined) {
    return {
      teacherId: a.teacherId, kind: a.kind ?? "", startsOn: a.startsOn, endsOn: a.endsOn, allDay: a.allDay,
      startsAt: a.startsAt ?? "", endsAt: a.endsAt ?? "", note: a.note ?? "",
    };
  }
  return { teacherId: p.teacherId ?? "", kind: "", startsOn: p.today, endsOn: p.today, allDay: true, startsAt: "", endsAt: "", note: "" };
}

/**
 * Register (or change) a teacher's absence (T19): who, why, which days and —
 * when it is not the whole day — which hours. Eight fields at most, so a
 * drawer (forms rule); the hours appear only when "Todo el día" is off. Its
 * kind and note are private to whoever manages the timetable: the gateway
 * never shows them to anybody else. A refusal from the server ("Ana Ruiz ya
 * tiene una ausencia…") is shown as is and the drawer stays open.
 */
export function AbsenceDrawer(props: AbsenceDrawerProps): ReactElement {
  const { teachers, cycleId, editing, onClose, onSaved } = props;
  const [v, setValues] = useState<Values>(() => initialValues(props));
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const [kindError, setKindError] = useState<string | undefined>(undefined);
  const [endsOnError, setEndsOnError] = useState<string | undefined>(undefined);
  const [endsAtError, setEndsAtError] = useState<string | undefined>(undefined);
  const required = useRequired(["teacherId", "startsOn", "endsOn", ...(v.allDay ? [] : ["startsAt", "endsAt"])]);
  const set = (patch: Partial<Values>): void => setValues((prev) => ({ ...prev, ...patch }));

  const valid = (): boolean => {
    const filled = required.check({ teacherId: v.teacherId, startsOn: v.startsOn, endsOn: v.endsOn, startsAt: v.startsAt, endsAt: v.endsAt });
    const kindOk = v.kind !== "";
    setKindError(kindOk ? undefined : "Elige el tipo de ausencia");
    const datesOk = v.startsOn === "" || v.endsOn === "" || v.endsOn >= v.startsOn;
    setEndsOnError(datesOk ? undefined : "La fecha final no puede ser antes de la inicial");
    const hoursOk = v.allDay || v.startsAt === "" || v.endsAt === "" || toMinutes(v.endsAt) > toMinutes(v.startsAt);
    setEndsAtError(hoursOk ? undefined : "La hora final debe ser después de la inicial");
    return filled && kindOk && datesOk && hoursOk;
  };

  const input = (): AbsenceInput => ({
    ...(cycleId !== undefined && editing === undefined ? { cycleId } : {}),
    teacherId: v.teacherId, kind: v.kind as AbsenceKind, startsOn: v.startsOn, endsOn: v.endsOn, allDay: v.allDay,
    ...(v.allDay ? {} : { startsAt: v.startsAt, endsAt: v.endsAt }),
    ...(v.note.trim() !== "" ? { note: v.note.trim() } : {}),
  });

  const submit = async (): Promise<void> => {
    if (pending || !valid()) return;
    setError(undefined);
    setPending(true);
    try {
      onSaved(editing === undefined ? await createAbsence(API_URL, input()) : await updateAbsence(API_URL, editing.id, input()));
    } catch (err) {
      setError(err instanceof Error ? err.message : "No se pudo guardar la ausencia.");
    } finally {
      setPending(false);
    }
  };

  // Cmd/Ctrl+Enter saves from any field (forms rule).
  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>): void => {
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      void submit();
    }
  };

  const teacherOptions = [...teachers]
    .sort((a, b) => a.lastName.localeCompare(b.lastName, "es-MX"))
    .map((t) => ({ value: t.id, label: `${t.firstName} ${t.lastName}` }));

  return (
    <SlideOver
      title={editing === undefined ? "Registrar ausencia" : "Editar ausencia"}
      subtitle="Sus clases de esas fechas quedan «Sin cubrir» hasta asignar suplente o cancelarlas"
      icon="calendar"
      submitLabel={editing === undefined ? "Registrar ausencia" : "Guardar cambios"}
      pending={pending}
      error={error}
      onClose={onClose}
      onSubmit={() => void submit()}
    >
      <div onKeyDown={onKeyDown} style={{ display: "grid", gap: 16, gridTemplateColumns: "minmax(0, 1fr)" }}>
        <SelectField label="Docente" required value={v.teacherId} disabled={editing !== undefined}
          error={required.errors.teacherId} onClearError={() => required.clear("teacherId")}
          help={editing === undefined ? "Quien faltará. Un docente no registra su propia ausencia." : "Una ausencia no cambia de docente; cancélala y registra otra."}
          onChange={(teacherId) => set({ teacherId })} options={teacherOptions} />
        <div style={{ display: "grid", gap: 6 }}>
          <ChipRow label="Tipo" options={KINDS} chosen={v.kind} showCounts={false}
            onChoose={(kind) => { set({ kind: kind as AbsenceKind }); setKindError(undefined); }} />
          {kindError !== undefined && <p role="alert" style={{ margin: 0, color: "var(--bad-fg)", fontSize: 13.5 }}>{kindError}</p>}
        </div>
        <div style={{ display: "grid", gap: 12, gridTemplateColumns: "repeat(auto-fit, minmax(min(100%, 160px), 1fr))" }}>
          <DateField label="Desde el" required value={v.startsOn} error={required.errors.startsOn} onClearError={() => required.clear("startsOn")}
            onChange={(startsOn) => set({ startsOn, endsOn: v.endsOn === "" || v.endsOn < startsOn ? startsOn : v.endsOn })} />
          <DateField label="Hasta el" required value={v.endsOn} error={required.errors.endsOn ?? endsOnError}
            onClearError={() => { required.clear("endsOn"); setEndsOnError(undefined); }} onChange={(endsOn) => set({ endsOn })} />
        </div>
        <label style={{ display: "flex", alignItems: "center", gap: 10, minHeight: 44, fontSize: 15, fontWeight: 600, color: "var(--text)", cursor: "pointer" }}>
          <input type="checkbox" checked={v.allDay} onChange={(e) => set({ allDay: e.target.checked })} style={{ width: 20, height: 20 }} />
          Todo el día
        </label>
        {!v.allDay && (
          <div style={{ display: "grid", gap: 12, gridTemplateColumns: "repeat(auto-fit, minmax(min(100%, 140px), 1fr))" }}>
            <SelectField label="Desde la hora" required value={v.startsAt} error={required.errors.startsAt}
              onClearError={() => required.clear("startsAt")} onChange={(startsAt) => set({ startsAt })} options={HOURS}
              help="Cada día de esas fechas; por ejemplo, una cita médica de 08:00 a 10:00." />
            <SelectField label="Hasta la hora" required value={v.endsAt} error={required.errors.endsAt ?? endsAtError}
              onClearError={() => { required.clear("endsAt"); setEndsAtError(undefined); }} onChange={(endsAt) => set({ endsAt })} options={HOURS} />
          </div>
        )}
        <TextArea label="Nota" value={v.note} rows={3} onChange={(note) => set({ note: note.slice(0, NOTE_MAX) })}
          help="Opcional. Solo la ve quien administra los horarios." />
      </div>
    </SlideOver>
  );
}

/** "Quedan 3 clases sin cubrir en esas fechas". */
export function uncoveredSentence(count: number): string {
  if (count === 0) return "No quedan clases sin cubrir en esas fechas";
  if (count === 1) return "Queda 1 clase sin cubrir en esas fechas";
  return `Quedan ${count} clases sin cubrir en esas fechas`;
}

/** What saving an absence left behind, with the shortcut to cover it. */
export function AbsenceNotice({ count, onCover, onDismiss }: { count: number; onCover?: () => void; onDismiss: () => void }): ReactElement {
  const pending = count > 0;
  return (
    <div role="status" style={{
      display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap", padding: "12px 16px", borderRadius: "var(--r-lg)",
      background: pending ? "var(--bad-bg)" : "var(--ok-bg)", color: pending ? "var(--bad-fg)" : "var(--ok-fg)", fontWeight: 600, fontSize: 15,
    }}>
      <span style={{ flex: "1 1 240px" }}>{uncoveredSentence(count)}</span>
      {pending && onCover !== undefined && (
        <button type="button" onClick={onCover} style={{ minHeight: 44, padding: "0 16px", borderRadius: 99, background: "var(--bad-fg)", color: "#fff", fontWeight: 700 }}>
          Cubrir ahora
        </button>
      )}
      <button type="button" onClick={onDismiss} aria-label="Cerrar aviso" style={{ minHeight: 44, minWidth: 44, color: "inherit", fontWeight: 700 }}>×</button>
    </div>
  );
}
