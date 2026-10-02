"use client";

import { useEffect, useState, type ReactElement } from "react";

import { API_URL } from "../../api-url";
import { listEvents, type ScheduleEvent } from "../../schedule/api";
import { SCHOOL_TIME_ZONE } from "../../schedule/time";
import { card, EmptyState } from "./parts";

const WHEN = new Intl.DateTimeFormat("es-MX", { timeZone: SCHOOL_TIME_ZONE, dateStyle: "medium", timeStyle: "short" });

export interface HistoryPanelProps {
  cycleId: string;
}

/** Who changed the timetable, when, what and why — newest first. */
export function HistoryPanel({ cycleId }: HistoryPanelProps): ReactElement {
  const [events, setEvents] = useState<ScheduleEvent[] | null>(null);
  const [failure, setFailure] = useState<string | undefined>(undefined);

  useEffect(() => {
    let live = true;
    listEvents(API_URL, cycleId)
      .then((got) => live && setEvents(got))
      .catch((err: unknown) => {
        if (!live) return;
        setEvents([]);
        setFailure(err instanceof Error ? err.message : "No se pudo leer el historial.");
      });
    return () => {
      live = false;
    };
  }, [cycleId]);

  if (failure !== undefined) return <p role="alert" style={{ margin: 0, color: "var(--bad-fg)", fontSize: 14.5 }}>{failure}</p>;
  if (events === null) return <p style={{ margin: 0, color: "var(--text-muted)" }}>Cargando historial…</p>;
  if (events.length === 0) {
    return <EmptyState>Aún no hay cambios registrados. Las suplencias, los intercambios y todo cambio hecho después de que empezó el ciclo aparecen aquí.</EmptyState>;
  }
  return (
    <ol aria-label="Historial de cambios" style={{ ...card, listStyle: "none", margin: 0, padding: "4px 0" }}>
      {events.map((e, i) => (
        <li key={e.id} style={{ display: "grid", gap: 4, padding: "14px 20px", borderTop: i === 0 ? "none" : "1px solid var(--border)" }}>
          <span style={{ color: "var(--text)", fontWeight: 600 }}>{e.summary}</span>
          <span style={{ color: "var(--text-muted)", fontSize: 13.5 }}>
            {e.actorName !== "" ? e.actorName : "Alguien sin nombre"} · {WHEN.format(new Date(e.at))}
          </span>
          {e.reason !== "" && <span style={{ color: "var(--text-muted)", fontSize: 13.5 }}>Motivo: {e.reason}</span>}
        </li>
      ))}
    </ol>
  );
}
