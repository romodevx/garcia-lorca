"use client";

import { useCallback, useEffect, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { SlideOver } from "../../_design/SlideOver";
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
import { DateField, TextField, useRequired } from "../../_design/form";
import { EmptyHint, ErrorAlert, Section } from "./parts";

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
/**
 * Los obligatorios, fuera del componente: `useRequired` los usa como
 * dependencia, y un arreglo nuevo en cada render reharía sus callbacks.
 */
const DAY_REQUIRED = ["label", "from"] as const;
const EVENT_REQUIRED = ["eventTitle", "eventFrom"] as const;

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
  // Nothing is created from the page itself: a closure and an activity each open
  // the same drawer the rest of the system uses.
  const [open, setOpen] = useState<"none" | "day" | "event">("none");

  const dayForm = useRequired(DAY_REQUIRED);
  const eventForm = useRequired(EVENT_REQUIRED);

  const reload = useCallback(() => {
    listNonWorkingPeriods(API_URL, cycle.id)
      .then(setPeriods)
      .catch(() => setError("No pudimos cargar el calendario."));
    listSchoolEvents(API_URL, cycle.id)
      .then(setEvents)
      .catch(() => setError("No pudimos cargar la agenda."));
  }, [cycle.id]);

  useEffect(reload, [reload]);

  const add = async (): Promise<void> => {
    // Lo que falta se marca en el propio campo, en rojo. El mensaje suelto
    // "necesita nombre y fecha" obligaba a adivinar cuál de los dos faltaba.
    if (!dayForm.check({ label, from })) return;
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
      setOpen("none");
      reload();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo agregar.");
    } finally {
      setSaving(false);
    }
  };

  const addEvent = async (): Promise<void> => {
    if (!eventForm.check({ eventTitle, eventFrom })) return;
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
      setOpen("none");
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

      <Btn icon="plus" onClick={() => { setError(undefined); setOpen("day"); }}>Nuevo día cerrado</Btn>
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

        <Btn icon="plus" onClick={() => { setError(undefined); setOpen("event"); }}>Nueva actividad</Btn>
      </div>

      {open === "none" && error !== undefined && <ErrorAlert message={error} />}

      {open === "day" && (
        <SlideOver
          title="Nuevo día cerrado"
          subtitle="Un feriado o un periodo de vacaciones. Mueve las fechas de pago."
          icon="calendar"
          submitLabel="Agregar día cerrado"
          pending={saving}
          error={error}
          onClose={() => setOpen("none")}
          onSubmit={() => void add()}>
          <TextField
            label="Nombre"
            value={label}
            onChange={setLabel}
            uppercase
            required
            placeholder="INDEPENDENCIA"
            error={dayForm.errors.label}
            onClearError={() => dayForm.clear("label")}
          />
          <DateField
            label="Del día"
            value={from}
            onChange={setFrom}
            required
            error={dayForm.errors.from}
            onClearError={() => dayForm.clear("from")}
          />
          <DateField label="Al día (opcional)" value={to} onChange={setTo} />
          <div>
            <p style={{ fontSize: 13, fontWeight: 600, color: "var(--text-muted)", marginBottom: 8 }}>Tipo</p>
            <div style={{ display: "flex", gap: 8 }}>
              {KINDS.map((k) => {
                const on = kind === k.value;
                return (
                  <button
                    key={k.value}
                    type="button"
                    onClick={() => setKind(k.value)}
                    style={{ height: 44, padding: "0 16px", borderRadius: 99, fontSize: 14, fontWeight: 600, cursor: "pointer", border: `1.5px solid ${on ? "var(--lime)" : "var(--border)"}`, background: on ? "color-mix(in oklab, var(--lime) 12%, var(--surface))" : "var(--surface)", color: on ? "var(--forest)" : "var(--text-muted)" }}>
                    {k.label}
                  </button>
                );
              })}
            </div>
          </div>
        </SlideOver>
      )}

      {open === "event" && (
        <SlideOver
          title="Nueva actividad"
          subtitle="Juntas, festivales, exámenes. No mueve fechas de pago."
          icon="calendar"
          submitLabel="Agregar actividad"
          pending={saving}
          error={error}
          onClose={() => setOpen("none")}
          onSubmit={() => void addEvent()}>
          <TextField
            label="Actividad"
            value={eventTitle}
            onChange={setEventTitle}
            uppercase
            required
            placeholder="FESTIVAL DE PRIMAVERA"
            error={eventForm.errors.eventTitle}
            onClearError={() => eventForm.clear("eventTitle")}
          />
          <DateField
            label="Fecha"
            value={eventFrom}
            onChange={setEventFrom}
            required
            error={eventForm.errors.eventFrom}
            onClearError={() => eventForm.clear("eventFrom")}
          />
          <DateField label="Hasta (opcional)" value={eventTo} onChange={setEventTo} />
        </SlideOver>
      )}
    </Section>
  );
}
