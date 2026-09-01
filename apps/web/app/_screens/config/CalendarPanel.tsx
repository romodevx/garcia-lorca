"use client";

import { useCallback, useEffect, useState, type FormEvent, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import {
  createNonWorkingPeriod,
  createSchoolEvent,
  deleteNonWorkingPeriod,
  deleteSchoolEvent,
  listNonWorkingPeriods,
  listSchoolEvents,
  type Cycle,
  type NonWorkingKind,
  type NonWorkingPeriod,
  type SchoolEvent,
} from "../../config/api";
import { EmptyHint, ErrorAlert, Section, TextField } from "./parts";

import { API_URL } from "../../api-url";

const KINDS: { value: NonWorkingKind; label: string }[] = [
  { value: "holiday", label: "Feriado" },
  { value: "vacation", label: "Vacaciones" },
];

export interface CalendarPanelProps {
  cycle: Cycle;
}

/**
 * The days the school does not operate.
 *
 * It is not decoration. The early-payment window is counted in WORKING days, so
 * a holiday nobody entered moves a family's deadline by a day — quietly, and in
 * the school's favour, which is the direction nobody notices until a parent
 * argues about it.
 *
 * A single closure is one date. Asking for the same day twice is how a calendar
 * stops being maintained.
 */
export function CalendarPanel({ cycle }: CalendarPanelProps): ReactElement {
  const [periods, setPeriods] = useState<NonWorkingPeriod[]>([]);
  const [events, setEvents] = useState<SchoolEvent[]>([]);
  const [eventTitle, setEventTitle] = useState("");
  const [eventFrom, setEventFrom] = useState("");
  const [eventTo, setEventTo] = useState("");
  const [label, setLabel] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [kind, setKind] = useState<NonWorkingKind>("holiday");
  const [error, setError] = useState<string | undefined>(undefined);
  const [saving, setSaving] = useState(false);

  const reload = useCallback(() => {
    listNonWorkingPeriods(API_URL, cycle.id)
      .then(setPeriods)
      .catch(() => setError("No pudimos cargar el calendario."));
    listSchoolEvents(API_URL, cycle.id)
      .then(setEvents)
      .catch(() => setError("No pudimos cargar la agenda."));
  }, [cycle.id]);

  useEffect(reload, [reload]);

  const add = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    if (label.trim() === "" || from.trim() === "") {
      setError("Un día cerrado necesita nombre y fecha.");
      return;
    }
    setSaving(true);
    setError(undefined);
    try {
      await createNonWorkingPeriod(API_URL, {
        cycleId: cycle.id,
        label: label.trim(),
        kind,
        startsOn: from.trim(),
        // Left out for a single day: the gateway fills it in from the start.
        ...(to.trim() !== "" ? { endsOn: to.trim() } : {}),
      });
      setLabel("");
      setFrom("");
      setTo("");
      reload();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo agregar.");
    } finally {
      setSaving(false);
    }
  };

  const addEvent = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    if (eventTitle.trim() === "" || eventFrom.trim() === "") {
      setError("Una actividad necesita nombre y fecha.");
      return;
    }
    setSaving(true);
    setError(undefined);
    try {
      await createSchoolEvent(API_URL, {
        cycleId: cycle.id,
        title: eventTitle.trim(),
        startsOn: eventFrom.trim(),
        ...(eventTo.trim() !== "" ? { endsOn: eventTo.trim() } : {}),
      });
      setEventTitle("");
      setEventFrom("");
      setEventTo("");
      reload();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo agregar la actividad.");
    } finally {
      setSaving(false);
    }
  };

  const removeEvent = async (event: SchoolEvent): Promise<void> => {
    try {
      await deleteSchoolEvent(API_URL, event.id);
      reload();
    } catch {
      setError("No se pudo quitar esa actividad.");
    }
  };

  const remove = async (period: NonWorkingPeriod): Promise<void> => {
    try {
      await deleteNonWorkingPeriod(API_URL, period.id);
      reload();
    } catch {
      setError("No se pudo quitar ese día.");
    }
  };

  return (
    <Section title="Calendario escolar" subtitle="Los días que la escuela no opera. El pronto pago se cuenta en días hábiles.">
      {periods.length === 0 && <EmptyHint>Todavía no hay días cerrados en este ciclo.</EmptyHint>}
      {periods.length > 0 && (
        <div style={{ borderRadius: "var(--r-md)", border: "1px solid var(--border)", overflow: "hidden", marginBottom: 16 }}>
          {periods.map((p) => (
            <div key={p.id} style={{ display: "flex", alignItems: "center", gap: 12, padding: "11px 16px", borderTop: "1px solid var(--border)" }}>
              <span style={{ fontSize: 15, fontWeight: 600, color: "var(--text)" }}>{p.label}</span>
              <span style={{ fontSize: 13.5, color: "var(--text-muted)" }}>
                {p.startsOn === p.endsOn ? p.startsOn : `${p.startsOn} → ${p.endsOn}`}
                {p.kind === "vacation" && " · vacaciones"}
              </span>
              <div style={{ flex: 1 }} />
              <button
                onClick={() => void remove(p)}
                aria-label={`Quitar ${p.label}`}
                style={{ background: "transparent", border: "none", color: "var(--text-muted)", fontSize: 13.5, fontWeight: 600, cursor: "pointer" }}>
                Quitar
              </button>
            </div>
          ))}
        </div>
      )}

      <form onSubmit={(e) => void add(e)} style={{ display: "flex", gap: 12, flexWrap: "wrap", alignItems: "flex-end" }}>
        <div style={{ flex: "2 1 200px" }}><TextField label="Nombre" value={label} onChange={setLabel} placeholder="Independencia" /></div>
        <div style={{ flex: "1 1 150px" }}><TextField label="Del día" value={from} onChange={setFrom} placeholder="2026-09-16" /></div>
        <div style={{ flex: "1 1 150px" }}><TextField label="Al día (opcional)" value={to} onChange={setTo} placeholder="2026-09-16" /></div>
        <div style={{ display: "flex", gap: 8 }}>
          {KINDS.map((k) => {
            const on = kind === k.value;
            return (
              <button
                key={k.value}
                type="button"
                onClick={() => setKind(k.value)}
                style={{ height: 44, padding: "0 14px", borderRadius: 99, fontSize: 14, fontWeight: 600, cursor: "pointer", border: `1.5px solid ${on ? "var(--lime)" : "var(--border)"}`, background: on ? "color-mix(in oklab, var(--lime) 12%, var(--surface))" : "var(--surface)", color: on ? "var(--forest)" : "var(--text-muted)" }}>
                {k.label}
              </button>
            );
          })}
        </div>
        <Btn type="submit" icon="plus" style={{ opacity: saving ? 0.5 : 1 }}>Agregar día cerrado</Btn>
      </form>
      <div style={{ borderTop: "1px solid var(--border)", marginTop: 22, paddingTop: 18 }}>
        <p style={{ fontSize: 15, fontWeight: 700, color: "var(--text)" }}>Actividades</p>
        <p style={{ fontSize: 13, color: "var(--text-faint)", marginTop: 2, marginBottom: 12 }}>
          Lo que hace la escuela: juntas, festivales, semanas de exámenes.
          A diferencia de los días cerrados, <b>no mueven fechas de pago</b>.
        </p>

        {events.length === 0 && <EmptyHint>Todavía no hay actividades en la agenda.</EmptyHint>}
        {events.length > 0 && (
          <div style={{ borderRadius: "var(--r-md)", border: "1px solid var(--border)", overflow: "hidden", marginBottom: 16 }}>
            {events.map((ev) => (
              <div key={ev.id} style={{ display: "flex", alignItems: "center", gap: 12, padding: "11px 16px", borderTop: "1px solid var(--border)" }}>
                <span style={{ fontSize: 15, fontWeight: 600, color: "var(--text)" }}>{ev.title}</span>
                <span style={{ fontSize: 13.5, color: "var(--text-muted)" }}>
                  {ev.startsOn === ev.endsOn ? ev.startsOn : `${ev.startsOn} → ${ev.endsOn}`}
                </span>
                <div style={{ flex: 1 }} />
                <button
                  onClick={() => void removeEvent(ev)}
                  aria-label={`Quitar ${ev.title}`}
                  style={{ background: "transparent", border: "none", color: "var(--text-muted)", fontSize: 13.5, fontWeight: 600, cursor: "pointer" }}>
                  Quitar
                </button>
              </div>
            ))}
          </div>
        )}

        <form onSubmit={(e) => void addEvent(e)} style={{ display: "flex", gap: 12, flexWrap: "wrap", alignItems: "flex-end" }}>
          <div style={{ flex: "2 1 200px" }}><TextField label="Actividad" value={eventTitle} onChange={setEventTitle} placeholder="Festival de primavera" /></div>
          <div style={{ flex: "1 1 150px" }}><TextField label="Fecha" value={eventFrom} onChange={setEventFrom} placeholder="2026-09-20" /></div>
          <div style={{ flex: "1 1 150px" }}><TextField label="Hasta (opcional)" value={eventTo} onChange={setEventTo} placeholder="2026-09-22" /></div>
          <Btn type="submit" icon="plus" style={{ opacity: saving ? 0.5 : 1 }}>Agregar actividad</Btn>
        </form>
      </div>

      {error !== undefined && <ErrorAlert message={error} />}
    </Section>
  );
}
