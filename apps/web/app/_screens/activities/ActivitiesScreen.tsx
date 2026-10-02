"use client";

import { useEffect, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import { Toast } from "../../_design/Toast";
import { localISODate } from "../../_lib/localDate";
import { API_URL } from "../../api-url";
import { listActivities, type Activity } from "../../activities/api";
import { StatCards } from "../uniforms/parts";
import { ActivitiesTable, type ActiveFilter } from "./ActivitiesTable";
import { ActivityEnrollPage } from "./ActivityEnrollPage";
import { ActivityFormPage } from "./ActivityFormPage";
import { useToast } from "./useToast";

const FAB_HEIGHT = 60;
const FAB_BOTTOM = 32;
const FAB_GAP = 16;

type Mode = { kind: "list" } | { kind: "form"; activity?: Activity } | { kind: "enroll"; activity: Activity };

export interface ActivitiesScreenProps {
  onBack: () => void;
  permissions?: readonly string[];
  /** The office's day (aaaa-mm-dd); injected by tests. */
  today?: string;
}

/**
 * Actividades (T17): the extracurriculars, their week and their price, and who
 * takes each. Reading needs activities:read; creating, editing and enrolling
 * need activities:write — the gateway decides, this only hides the doors.
 */
export function ActivitiesScreen({ onBack, permissions, today: injectedToday }: ActivitiesScreenProps): ReactElement {
  const today = injectedToday ?? localISODate();
  const canWrite = (permissions ?? []).includes("activities:write");
  const [mode, setMode] = useState<Mode>({ kind: "list" });
  const [filter, setFilter] = useState<ActiveFilter>("active");
  const [activities, setActivities] = useState<Activity[] | null>(null);
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const [reads, setReads] = useState(0);
  const { toast, say } = useToast();

  useEffect(() => {
    let live = true;
    setFailure(undefined);
    listActivities(API_URL, true)
      .then((list) => live && setActivities(list))
      .catch((err: unknown) => {
        if (!live) return;
        setActivities([]);
        setFailure(err instanceof Error ? err.message : "No se pudieron leer las actividades.");
      });
    return () => {
      live = false;
    };
  }, [reads]);

  const backToList = (): void => {
    setMode({ kind: "list" });
    setReads((n) => n + 1);
  };

  if (mode.kind === "form") {
    return <ActivityFormPage activity={mode.activity} today={today} onClose={() => setMode({ kind: "list" })} onSaved={(message) => { say(message); backToList(); }} />;
  }
  if (mode.kind === "enroll") {
    return <ActivityEnrollPage activity={mode.activity} canWrite={canWrite} onBack={backToList} />;
  }

  const all = activities ?? [];
  const active = all.filter((a) => a.active);
  const shown = filter === "active" ? active : all.filter((a) => !a.active);
  const enrolled = active.reduce((sum, a) => sum + a.enrolledCount, 0);
  const cards = [
    { icon: "star" as const, color: "yellow" as const, label: "Actividades activas", value: String(active.length), sub: "se pueden inscribir", testId: "stat-active" },
    { icon: "students" as const, color: "forest" as const, label: "Alumnos inscritos", value: String(enrolled), sub: "en actividades activas", testId: "stat-enrolled" },
    { icon: "calendar" as const, color: "purple" as const, label: "Inactivas", value: String(all.length - active.length), sub: "no se cobran", testId: "stat-inactive" },
  ];
  const create = (): void => setMode({ kind: "form" });

  return (
    <Page>
      <button onClick={onBack} style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14 }}>
        <Icon name="back" size={18} stroke={2.4} />
        Volver al inicio
      </button>

      <header style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 24, flexWrap: "wrap" }}>
        <div style={{ width: 56, height: 56, borderRadius: 18, display: "grid", placeItems: "center", background: "var(--forest)", color: "#fff", boxShadow: "0 6px 16px color-mix(in oklab, var(--forest) 38%, transparent)", flexShrink: 0 }}>
          <Icon name="star" size={30} />
        </div>
        <div style={{ minWidth: 0, flex: 1 }}>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Actividades</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>Extracurriculares y su cobro</p>
        </div>
        {canWrite && <Btn icon="plus" onClick={create}>Nueva actividad</Btn>}
      </header>

      <div style={{ display: "grid", gap: 18 }}>
        {failure !== undefined && <p role="alert" style={{ margin: 0, color: "var(--bad-fg)", fontSize: 14.5 }}>{failure}</p>}
        <StatCards cards={cards} />
        <ActivitiesTable
          rows={shown}
          loading={activities === null}
          filter={filter}
          counts={{ active: active.length, inactive: all.length - active.length }}
          onFilter={setFilter}
          canWrite={canWrite}
          onEdit={(activity) => setMode({ kind: "form", activity })}
          onEnroll={(activity) => setMode({ kind: "enroll", activity })}
        />
      </div>

      {canWrite && (
        <>
          <div aria-hidden style={{ height: FAB_BOTTOM + FAB_HEIGHT + FAB_GAP }} />
          <button onClick={create} title="Nueva actividad" aria-label="Nueva actividad"
            style={{ position: "fixed", right: 32, bottom: FAB_BOTTOM, zIndex: 50, height: FAB_HEIGHT, padding: "0 26px", borderRadius: 99, display: "inline-flex", alignItems: "center", gap: 10, background: "var(--primary)", color: "var(--primary-ink)", fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 16.5, cursor: "pointer", boxShadow: "0 10px 30px color-mix(in oklab, var(--lime) 45%, transparent)" }}>
            <Icon name="plus" size={22} stroke={2.6} />Nueva actividad
          </button>
        </>
      )}
      {toast !== undefined && <Toast>{toast}</Toast>}
    </Page>
  );
}
