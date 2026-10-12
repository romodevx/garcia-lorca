"use client";

import { useEffect, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import { Toast } from "../../_design/Toast";
import { API_URL } from "../../api-url";
import { listScholarships, listScholarshipTargets, type Scholarship, type ScholarshipTarget } from "../../scholarships/api";
import { useIntent } from "../../routing/pendingIntent";
import { useToast } from "../activities/useToast";
import { StatCards } from "../uniforms/parts";
import { ScholarshipAssignPage } from "./ScholarshipAssignPage";
import { ScholarshipDrawer } from "./ScholarshipDrawer";
import { ScholarshipsTable, type ActiveFilter } from "./ScholarshipsTable";
import { ModuleHeaderIcon } from "../../_design/ModuleHeaderIcon";

const FAB_HEIGHT = 60;
const FAB_BOTTOM = 32;
const FAB_GAP = 16;

/** The drawer open on a new scholarship (no `scholarship`) or on one to edit. */
type Editing = { scholarship?: Scholarship } | null;

export interface ScholarshipsScreenProps {
  onBack: () => void;
  permissions?: readonly string[];
}

/**
 * Becas (T18): what each scholarship takes off and from what, and who holds
 * it. Reading needs scholarships:read; creating, editing and assigning need
 * scholarships:write — the gateway decides, this only hides the doors.
 */
export function ScholarshipsScreen({ onBack, permissions }: ScholarshipsScreenProps): ReactElement {
  const canWrite = (permissions ?? []).includes("scholarships:write");
  const [assigning, setAssigning] = useState<Scholarship | null>(null);
  const intent = useIntent("scholarships");
  const [editing, setEditing] = useState<Editing>(() => (intent === "new" && canWrite ? {} : null));
  const [filter, setFilter] = useState<ActiveFilter>("active");
  const [scholarships, setScholarships] = useState<Scholarship[] | null>(null);
  const [targets, setTargets] = useState<ScholarshipTarget[]>([]);
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const [reads, setReads] = useState(0);
  const { toast, say } = useToast();

  useEffect(() => {
    let live = true;
    setFailure(undefined);
    Promise.all([listScholarships(API_URL, true), listScholarshipTargets(API_URL)])
      .then(([list, catalog]) => {
        if (!live) return;
        setScholarships(list);
        setTargets(catalog);
      })
      .catch((err: unknown) => {
        if (!live) return;
        setScholarships([]);
        setFailure(err instanceof Error ? err.message : "No se pudieron leer las becas.");
      });
    return () => {
      live = false;
    };
  }, [reads]);

  const reload = (): void => setReads((n) => n + 1);

  if (assigning !== null) {
    return <ScholarshipAssignPage scholarship={assigning} targets={targets} canWrite={canWrite} onBack={() => { setAssigning(null); reload(); }} />;
  }

  const all = scholarships ?? [];
  const active = all.filter((s) => s.active);
  const shown = filter === "active" ? active : all.filter((s) => !s.active);
  const holders = active.reduce((sum, s) => sum + s.assignedCount, 0);
  const cards = [
    { icon: "scholarship" as const, color: "purple" as const, label: "Becas activas", value: String(active.length), sub: "se pueden asignar", testId: "stat-active" },
    { icon: "students" as const, color: "forest" as const, label: "Alumnos con beca", value: String(holders), sub: "sumando las becas activas", testId: "stat-holders" },
    { icon: "calendar" as const, color: "neutral" as const, label: "Inactivas", value: String(all.length - active.length), sub: "no se aplican", testId: "stat-inactive" },
  ];
  const create = (): void => setEditing({});

  return (
    <Page>
      <button onClick={onBack} style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14 }}>
        <Icon name="back" size={18} stroke={2.4} />
        Volver al inicio
      </button>

      <header style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 24, flexWrap: "wrap" }}>
        <ModuleHeaderIcon icon="scholarship" color="purple" />
        <div style={{ minWidth: 0, flex: 1 }}>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Becas</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>Becas y descuentos, y quién los tiene</p>
        </div>
        {canWrite && <Btn icon="plus" onClick={create}>Nueva beca</Btn>}
      </header>

      <div style={{ display: "grid", gap: 18 }}>
        {failure !== undefined && <p role="alert" style={{ margin: 0, color: "var(--bad-fg)", fontSize: 14.5 }}>{failure}</p>}
        <StatCards cards={cards} />
        <ScholarshipsTable
          rows={shown}
          targets={targets}
          loading={scholarships === null}
          filter={filter}
          counts={{ active: active.length, inactive: all.length - active.length }}
          onFilter={setFilter}
          canWrite={canWrite}
          onEdit={(scholarship) => setEditing({ scholarship })}
          onAssign={setAssigning}
        />
      </div>

      {canWrite && (
        <>
          <div aria-hidden style={{ height: FAB_BOTTOM + FAB_HEIGHT + FAB_GAP }} />
          <button onClick={create} title="Nueva beca" aria-label="Nueva beca"
            style={{ position: "fixed", right: 32, bottom: FAB_BOTTOM, zIndex: 50, height: FAB_HEIGHT, padding: "0 26px", borderRadius: 99, display: "inline-flex", alignItems: "center", gap: 10, background: "var(--primary)", color: "var(--primary-ink)", fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 16.5, cursor: "pointer", boxShadow: "0 10px 30px color-mix(in oklab, var(--lime) 45%, transparent)" }}>
            <Icon name="plus" size={22} stroke={2.6} />Nueva beca
          </button>
        </>
      )}
      {editing !== null && (
        <ScholarshipDrawer scholarship={editing.scholarship} targets={targets} onClose={() => setEditing(null)}
          onSaved={(message) => { setEditing(null); say(message); reload(); }} />
      )}
      {toast !== undefined && <Toast>{toast}</Toast>}
    </Page>
  );
}
