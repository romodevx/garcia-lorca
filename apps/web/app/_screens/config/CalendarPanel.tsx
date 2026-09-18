"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type ReactElement } from "react";

import { InlineCreateRow } from "../../_design/InlineCreateRow";
import { UndoToast } from "../../_design/UndoToast";
import { useUndoableRemoval } from "../../_design/useUndoableRemoval";
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
import { EmptyHint, ErrorAlert, Section } from "./parts";

import { API_URL } from "../../api-url";

/**
 * Los días que la escuela no opera.
 *
 * No es decoración. La ventana de pronto pago se cuenta en días HÁBILES, así que
 * un feriado que nadie capturó mueve el plazo de una familia por un día — en
 * silencio y a favor de la escuela, que es la dirección que nadie nota hasta que
 * un papá reclama.
 *
 * Por eso se captura en renglón y no en cajón: un calendario que cuesta doce
 * viajes es un calendario que se deja a medias, y dejarlo a medias sale caro.
 */
const KIND_OPTIONS: readonly { value: NonWorkingKind; label: string }[] = [
  { value: "holiday", label: "Feriado" },
  { value: "vacation", label: "Vacaciones" },
];

export interface CalendarPanelProps {
  cycle: Cycle;
}

const rowStyle = {
  display: "flex",
  alignItems: "center",
  gap: 12,
  padding: "11px 16px",
  borderTop: "1px solid var(--border)",
} as const;

const removeStyle = {
  background: "transparent",
  border: "none",
  color: "var(--text-muted)",
  fontSize: 13.5,
  fontWeight: 600,
  cursor: "pointer",
} as const;

export function CalendarPanel({ cycle }: CalendarPanelProps): ReactElement {
  const [periods, setPeriods] = useState<NonWorkingPeriod[]>([]);
  const [events, setEvents] = useState<SchoolEvent[]>([]);
  const [error, setError] = useState<string | undefined>(undefined);
  /**
   * Lo quitado y todavía deshacible. El plazo lo lleva el hook: guardar un solo
   * pendiente aquí hacía que quitar dos días seguidos perdiera el borrado del
   * primero — desaparecía de la lista y seguía en el servidor.
   */
  /*
   * Lo que está en el limbo se filtra de TODO lo que llega del servidor.
   *
   * La lista de allá todavía trae el renglón quitado, así que una recarga dentro
   * del plazo lo resucitaba en pantalla —con el toast de Deshacer aún visible— y
   * ocho segundos después se iba solo. Peor: darle "Quitar" al resucitado
   * encolaba un segundo borrado del mismo id, que falla.
   *
   * En refs para que `reload` no cambie de identidad en cada quitada: es
   * dependencia de un efecto.
   */
  const pendingDays = useRef<readonly NonWorkingPeriod[]>([]);
  const pendingEvents = useRef<readonly SchoolEvent[]>([]);

  const dayRemoval = useUndoableRemoval<NonWorkingPeriod>();
  const eventRemoval = useUndoableRemoval<SchoolEvent>();
  pendingDays.current = dayRemoval.pending;
  pendingEvents.current = eventRemoval.pending;

  const reload = useCallback(() => {
    listNonWorkingPeriods(API_URL, cycle.id)
      .then((rows) => setPeriods(rows.filter((r) => !pendingDays.current.some((p) => p.id === r.id))))
      .catch(() => setError("No pudimos cargar el calendario."));
    listSchoolEvents(API_URL, cycle.id)
      .then((rows) => setEvents(rows.filter((r) => !pendingEvents.current.some((p) => p.id === r.id))))
      .catch(() => setError("No pudimos cargar la agenda."));
  }, [cycle.id]);

  useEffect(reload, [reload]);

  // Un día cerrado que termina antes de empezar no es un dato conservador: es un
  // dato que mueve mal las fechas de pago. Se revisa al salir de la celda.
  const dayRules = useMemo(
    () => ({
      endsOn: (value: string, all: Readonly<Record<string, string>>): string | undefined => {
        const from = all.startsOn ?? "";
        return from !== "" && value < from ? "El último día es antes que el primero." : undefined;
      },
    }),
    [],
  );

  const eventRules = useMemo(
    () => ({
      endsOn: (value: string, all: Readonly<Record<string, string>>): string | undefined => {
        const from = all.startsOn ?? "";
        return from !== "" && value < from ? "La fecha final es antes que la inicial." : undefined;
      },
    }),
    [],
  );

  const dayDefaults = useMemo(() => ({ kind: "holiday" }), []);

  const addDay = async (values: Record<string, string>): Promise<void> => {
    setError(undefined);
    try {
      await createNonWorkingPeriod(API_URL, {
        cycleId: cycle.id,
        label: (values.label ?? "").trim(),
        kind: (values.kind ?? "holiday") as NonWorkingKind,
        startsOn: (values.startsOn ?? "").trim(),
        // Se omite para un solo día: el gateway lo completa desde el inicio.
        ...((values.endsOn ?? "").trim() !== "" ? { endsOn: values.endsOn } : {}),
      });
      reload();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo agregar.");
      // Relanzado a propósito: el renglón conserva lo escrito sólo si se entera.
      throw cause;
    }
  };

  const addEvent = async (values: Record<string, string>): Promise<void> => {
    setError(undefined);
    try {
      await createSchoolEvent(API_URL, {
        cycleId: cycle.id,
        title: (values.title ?? "").trim(),
        startsOn: (values.startsOn ?? "").trim(),
        ...((values.endsOn ?? "").trim() !== "" ? { endsOn: values.endsOn } : {}),
      });
      reload();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo agregar la actividad.");
      throw cause;
    }
  };

  return (
    <Section
      title="Calendario escolar"
      subtitle="Los días que la escuela no opera. El pronto pago se cuenta en días hábiles."
    >
      {periods.length === 0 && <EmptyHint>Todavía no hay días cerrados en este ciclo.</EmptyHint>}
      <div style={{ borderRadius: "var(--r-md)", border: "1px solid var(--border)", overflow: "hidden" }}>
        {periods.map((p) => (
          <div key={p.id} style={rowStyle}>
            <span style={{ fontSize: 15, fontWeight: 600, color: "var(--text)" }}>{p.label}</span>
            <span style={{ fontSize: 13.5, color: "var(--text-muted)" }}>
              {p.startsOn === p.endsOn ? p.startsOn : `${p.startsOn} → ${p.endsOn}`}
              {p.kind === "vacation" && " · vacaciones"}
            </span>
            <div style={{ flex: 1 }} />
            <button
              onClick={() => {
                setPeriods((prev) => prev.filter((x) => x.id !== p.id));
                dayRemoval.remove(p, () => {
                  void deleteNonWorkingPeriod(API_URL, p.id)
                    .then(reload)
                    .catch(() => setError("No se pudo quitar ese día."));
                });
              }}
              aria-label={`Quitar ${p.label}`}
              style={removeStyle}
            >
              Quitar
            </button>
          </div>
        ))}
        <InlineCreateRow
          addLabel="Nuevo día cerrado"
          defaults={dayDefaults}
          rules={dayRules}
          columns={[
            { key: "label", label: "Nombre", kind: "text", uppercase: true, required: true, placeholder: "INDEPENDENCIA" },
            { key: "startsOn", label: "Del día", kind: "date", required: true, width: 160 },
            { key: "endsOn", label: "Al día (opcional)", kind: "date", width: 160 },
            { key: "kind", label: "Tipo", kind: "select", options: KIND_OPTIONS, width: 140 },
          ]}
          onCreate={addDay}
        />
      </div>

      <div style={{ borderTop: "1px solid var(--border)", marginTop: 22, paddingTop: 18 }}>
        <p style={{ fontSize: 15, fontWeight: 700, color: "var(--text)" }}>Actividades</p>
        <p style={{ fontSize: 13, color: "var(--text-faint)", marginTop: 2, marginBottom: 12 }}>
          Lo que hace la escuela: juntas, festivales, semanas de exámenes.
          A diferencia de los días cerrados, <b>no mueven fechas de pago</b>.
        </p>

        {events.length === 0 && <EmptyHint>Todavía no hay actividades en la agenda.</EmptyHint>}
        <div style={{ borderRadius: "var(--r-md)", border: "1px solid var(--border)", overflow: "hidden" }}>
          {events.map((ev) => (
            <div key={ev.id} style={rowStyle}>
              <span style={{ fontSize: 15, fontWeight: 600, color: "var(--text)" }}>{ev.title}</span>
              <span style={{ fontSize: 13.5, color: "var(--text-muted)" }}>
                {ev.startsOn === ev.endsOn ? ev.startsOn : `${ev.startsOn} → ${ev.endsOn}`}
              </span>
              <div style={{ flex: 1 }} />
              <button
                onClick={() => {
                  setEvents((prev) => prev.filter((x) => x.id !== ev.id));
                  eventRemoval.remove(ev, () => {
                    void deleteSchoolEvent(API_URL, ev.id)
                      .then(reload)
                      .catch(() => setError("No se pudo quitar esa actividad."));
                  });
                }}
                aria-label={`Quitar ${ev.title}`}
                style={removeStyle}
              >
                Quitar
              </button>
            </div>
          ))}
          <InlineCreateRow
            addLabel="Nueva actividad"
            rules={eventRules}
            columns={[
              { key: "title", label: "Actividad", kind: "text", uppercase: true, required: true, placeholder: "FESTIVAL DE PRIMAVERA" },
              { key: "startsOn", label: "Fecha", kind: "date", required: true, width: 160 },
              { key: "endsOn", label: "Hasta (opcional)", kind: "date", width: 160 },
            ]}
            onCreate={addEvent}
          />
        </div>
      </div>

      {error !== undefined && <ErrorAlert message={error} />}

      {dayRemoval.latest !== undefined && (
        <UndoToast
          message={`Se quitó ${dayRemoval.latest.label}`}
          onUndo={() => {
            const devueltos = dayRemoval.undo() ?? [];
            setPeriods((prev) =>
              [...prev, ...devueltos].sort((a, b) => a.startsOn.localeCompare(b.startsOn)),
            );
          }}
        />
      )}

      {eventRemoval.latest !== undefined && (
        <UndoToast
          message={`Se quitó ${eventRemoval.latest.title}`}
          onUndo={() => {
            const devueltas = eventRemoval.undo() ?? [];
            setEvents((prev) =>
              [...prev, ...devueltas].sort((a, b) => a.startsOn.localeCompare(b.startsOn)),
            );
          }}
        />
      )}
    </Section>
  );
}
