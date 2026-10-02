"use client";

import { useState, type ReactElement } from "react";

import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import { SelectField } from "../../_design/form";
import { HistoryPanel } from "./HistoryPanel";
import { NowView } from "./NowView";
import { Tabs } from "./parts";
import { sourceFor, useScheduleBoard, type ScheduleMode } from "./useScheduleBoard";
import { WeekView } from "./WeekView";

type TabId = "week" | "now" | "history";

export interface ScheduleScreenProps {
  onBack: () => void;
  mode: ScheduleMode;
  /** schedule:write. UX only: the gateway refuses the write without it. */
  canWrite: boolean;
  /** The instant "now" means; injected by tests. */
  now?: Date;
}

/**
 * Horarios (T16): the week of a group, a grade or a teacher (Monday–Friday on
 * a 5-minute grid), what is happening right now, and who changed what. In
 * "teacher" mode it is the caller's own timetable, read-only.
 */
export function ScheduleScreen({ onBack, mode, canWrite, now }: ScheduleScreenProps): ReactElement {
  const [source] = useState(() => sourceFor(mode));
  const [cycleId, setCycleId] = useState<string | undefined>(undefined);
  const { board, failure, reload } = useScheduleBoard(source, cycleId);
  const [tab, setTab] = useState<TabId>("week");
  const manages = mode === "manage";
  const editable = manages && canWrite;

  const tabs: { id: TabId; label: string }[] = [
    { id: "week", label: manages ? "Semana" : "Mi semana" },
    { id: "now", label: "¿Qué está pasando ahora?" },
    ...(manages ? [{ id: "history" as const, label: "Historial" }] : []),
  ];

  return (
    <Page>
      <button onClick={onBack} style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14 }}>
        <Icon name="back" size={18} stroke={2.4} />
        Volver al inicio
      </button>

      <header style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 24, flexWrap: "wrap" }}>
        <div style={{ width: 56, height: 56, borderRadius: 18, display: "grid", placeItems: "center", background: "var(--forest)", color: "#fff", boxShadow: "0 6px 16px color-mix(in oklab, var(--forest) 38%, transparent)", flexShrink: 0 }}>
          <Icon name="clock" size={30} />
        </div>
        <div style={{ minWidth: 0, flex: 1 }}>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>{manages ? "Horarios" : "Mi horario"}</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>
            {board === null ? "Cargando horario…" : `Ciclo ${board.cycle.name} · ${board.blocks.length} ${board.blocks.length === 1 ? "clase" : "clases"} a la semana`}
          </p>
        </div>
        {manages && board !== null && board.cycles.length > 1 && (
          <div style={{ width: 240, maxWidth: "100%" }}>
            <SelectField
              label="Ciclo"
              value={board.cycle.id}
              onChange={(id) => setCycleId(id === "" ? undefined : id)}
              options={board.cycles.map((c) => ({ value: c.id, label: c.active ? `${c.name} (activo)` : c.name }))}
            />
          </div>
        )}
      </header>

      {failure !== undefined && <p role="alert" style={{ margin: "0 0 18px", color: "var(--bad-fg)", fontSize: 14.5 }}>{failure}</p>}

      {board !== null && (
        <div style={{ display: "grid", gap: 18, gridTemplateColumns: "minmax(0, 1fr)" }}>
          <Tabs label="Secciones de horarios" tabs={tabs} active={tab} onChoose={setTab} />
          {tab === "week" && <WeekView board={board} mode={mode} canWrite={editable} now={now} onChanged={reload} />}
          {tab === "now" && <NowView board={board} source={source} canWrite={editable} now={now} />}
          {tab === "history" && <HistoryPanel cycleId={board.cycle.id} />}
        </div>
      )}
    </Page>
  );
}
