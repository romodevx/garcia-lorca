"use client";

import { useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import { SelectField } from "../../_design/form";
import type { AbsenceSaved, TeacherAbsence } from "../../schedule/api";
import { nowInSchoolZone } from "../../schedule/time";
import { AbsenceDrawer, AbsenceNotice } from "./AbsenceDrawer";
import { HistoryPanel } from "./HistoryPanel";
import { NowView } from "./NowView";
import { Tabs } from "./parts";
import { rangeForAbsence, UncoveredView, type UncoveredRange } from "./UncoveredView";
import { sourceFor, useScheduleBoard, type ScheduleMode } from "./useScheduleBoard";
import { WeekView } from "./WeekView";
import { ModuleHeaderIcon } from "../../_design/ModuleHeaderIcon";

export type TabId = "week" | "now" | "uncovered" | "history";

/** The absence drawer: closed, a new absence, or one being edited. */
type AbsenceTarget = null | { kind: "new" } | { kind: "edit"; absence: TeacherAbsence };

export interface ScheduleScreenProps {
  onBack: () => void;
  mode: ScheduleMode;
  /** schedule:write. UX only: the gateway refuses the write without it. */
  canWrite: boolean;
  /** The instant "now" means; injected by tests. */
  now?: Date;
  /** An absence saved elsewhere (Docentes, "Cubrir ahora"): open on "Por cubrir", on its dates. */
  focus?: { startsOn: string; endsOn: string };
  /** Open on the new-absence drawer ("Registrar ausencia" shortcut); only where it could be opened by hand. */
  openAbsence?: boolean;
}

/**
 * Horarios (T16): the week of a group, a grade or a teacher (Monday–Friday on
 * a 5-minute grid), what is happening right now, and who changed what. In
 * "teacher" mode it is the caller's own timetable, read-only.
 */
export function ScheduleScreen({ onBack, mode, canWrite, now, focus, openAbsence = false }: ScheduleScreenProps): ReactElement {
  const [source] = useState(() => sourceFor(mode));
  const [cycleId, setCycleId] = useState<string | undefined>(undefined);
  const { board, failure, reload } = useScheduleBoard(source, cycleId);
  const manages = mode === "manage";
  const [tab, setTab] = useState<TabId>(manages && focus !== undefined ? "uncovered" : "week");
  const editable = manages && canWrite;
  const today = nowInSchoolZone(now).date;
  const [range, setRange] = useState<UncoveredRange>(() => (focus !== undefined ? rangeForAbsence(focus, today) : { kind: "today" }));
  const [absenceTarget, setAbsenceTarget] = useState<AbsenceTarget>(() => (openAbsence && editable ? { kind: "new" } : null));
  const [saved, setSaved] = useState<AbsenceSaved | null>(null);
  // Bumped on every absence saved: the week and "Por cubrir" read again.
  const [absenceVersion, setAbsenceVersion] = useState(0);

  const tabs: { id: TabId; label: string }[] = [
    { id: "week", label: manages ? "Semana" : "Mi semana" },
    { id: "now", label: "¿Qué está pasando ahora?" },
    ...(manages ? [{ id: "uncovered" as const, label: "Por cubrir" }, { id: "history" as const, label: "Historial" }] : []),
  ];

  const absenceSaved = (got: AbsenceSaved): void => {
    setAbsenceTarget(null);
    setSaved(got);
    setAbsenceVersion((n) => n + 1);
  };

  // "Cubrir ahora": the uncovered classes of the absence's dates.
  const coverNow = (): void => {
    if (saved === null) return;
    setRange(rangeForAbsence(saved.absence, today));
    setTab("uncovered");
    setSaved(null);
  };

  return (
    <Page>
      <button onClick={onBack} style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14 }}>
        <Icon name="back" size={18} stroke={2.4} />
        Volver al inicio
      </button>

      <header style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 24, flexWrap: "wrap" }}>
        <ModuleHeaderIcon icon="clock" color="forest" />
        <div style={{ minWidth: 0, flex: 1 }}>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>{manages ? "Horarios" : "Mi horario"}</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>
            {board === null ? "Cargando horario…" : `Ciclo ${board.cycle.name} · ${board.blocks.length} ${board.blocks.length === 1 ? "clase" : "clases"} a la semana`}
          </p>
        </div>
        {editable && board !== null && (
          <Btn icon="calendar" kind="outline" onClick={() => setAbsenceTarget({ kind: "new" })}>Registrar ausencia</Btn>
        )}
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
          {saved !== null && <AbsenceNotice count={saved.uncoveredCount} onCover={coverNow} onDismiss={() => setSaved(null)} />}
          <Tabs label="Secciones de horarios" tabs={tabs} active={tab} onChoose={setTab} />
          {tab === "week" && <WeekView board={board} mode={mode} canWrite={editable} now={now} onChanged={reload} version={absenceVersion} />}
          {tab === "now" && <NowView board={board} source={source} canWrite={editable} now={now} />}
          {tab === "uncovered" && (
            <UncoveredView board={board} canWrite={editable} today={today} range={range} onRange={setRange} version={absenceVersion}
              onEditAbsence={(absence) => setAbsenceTarget({ kind: "edit", absence })} />
          )}
          {tab === "history" && <HistoryPanel cycleId={board.cycle.id} />}
        </div>
      )}
      {absenceTarget !== null && board !== null && (
        <AbsenceDrawer teachers={board.teachers} cycleId={board.cycle.id} today={today}
          editing={absenceTarget.kind === "edit" ? absenceTarget.absence : undefined}
          onClose={() => setAbsenceTarget(null)} onSaved={absenceSaved} />
      )}
    </Page>
  );
}
