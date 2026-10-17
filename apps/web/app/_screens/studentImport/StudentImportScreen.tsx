"use client";

import { useEffect, useRef, useState, type ReactElement } from "react";

import { API_URL } from "../../api-url";
import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import { Stepper, type StepDef } from "../../_design/Stepper";
import { localISODate } from "../../_lib/localDate";
import { listLevels, type Level } from "../../config/api";
import { busyText } from "../../studentImport/copy";
import { CycleStep } from "./CycleStep";
import { PreviewStep } from "./PreviewStep";
import { ResultStep } from "./ResultStep";
import { UploadStep } from "./UploadStep";
import { useImportCycle } from "./useImportCycle";
import { useStudentImport, type ImportPhase } from "./useStudentImport";
import { alertStyle, cardStyle, noteStyle } from "./parts";

type StepId = "cycle" | "upload" | "preview" | "result";

const FLOW_STEPS: readonly StepDef<StepId>[] = [
  { id: "upload", label: "Subir" },
  { id: "preview", label: "Vista previa" },
  { id: "result", label: "Resultado" },
];
const CYCLE_STEP: StepDef<StepId> = { id: "cycle", label: "Ciclo" };
const TITLE: Record<StepId, string> = { cycle: "Ciclo escolar", upload: "Sube el archivo", preview: "Vista previa", result: "Resultado" };
const STEP_OF_PHASE: Record<ImportPhase, StepId> = { upload: "upload", preview: "preview", committing: "result", result: "result" };

export interface StudentImportScreenProps {
  /** Back to Configuración, nothing written. */
  onBack: () => void;
  /** Colegiaturas lives in Configuración. */
  onOpenTuition: () => void;
  /** Estudiantes, filtered by the level when it is known. */
  onViewStudents: (levelId: string | undefined) => void;
  /** Today, YYYY-MM-DD; injected by tests. */
  today?: string;
}

/**
 * Migrating pupils from the school's previous system (ADR 0033): own page, a
 * stepper in forest/lime, focus on each step's heading. The gateway
 * (students:import + three flags) is the boundary; this screen is UX.
 */
export function StudentImportScreen({ onBack, onOpenTuition, onViewStudents, today = localISODate() }: StudentImportScreenProps): ReactElement {
  const cycle = useImportCycle(API_URL);
  const flow = useStudentImport(API_URL, cycle.reload);
  const levels = useSchoolLevels(flow.preview);
  const heading = useRef<HTMLHeadingElement | null>(null);
  const step: StepId = cycle.state === "ready" && cycle.active === undefined ? "cycle" : STEP_OF_PHASE[flow.phase];
  const committing = flow.phase === "committing";

  useFocusOnChange(heading, cycle.state === "ready" ? step : undefined);
  useWarnBeforeLeaving(committing);

  return (
    <Page variant="wide">
      <button onClick={onBack} style={{ display: "inline-flex", alignItems: "center", gap: 7, minHeight: 44, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 6 }}>
        <Icon name="back" size={18} stroke={2.4} />
        Volver a Configuración
      </button>
      <h1 style={{ fontSize: 28, fontWeight: 800, color: "var(--text)" }}>Migrar alumnos</h1>
      {cycle.active !== undefined && <p style={{ fontSize: 15, fontWeight: 600, color: "var(--forest)", margin: "4px 0 14px" }}>{`Destino: Ciclo ${cycle.active.name}`}</p>}
      <Stepper steps={step === "cycle" ? [CYCLE_STEP, ...FLOW_STEPS] : FLOW_STEPS} current={step} tone="forest" />

      <section style={cardStyle}>
        <h2 ref={heading} tabIndex={-1} style={{ fontSize: 19, fontWeight: 700, color: "var(--text)" }}>{TITLE[step]}</h2>
        {cycle.state === "loading" && <p style={{ color: "var(--text-muted)" }}>Cargando…</p>}
        {cycle.state === "failed" && <p role="alert" style={alertStyle}>No pudimos leer los ciclos escolares. Revisa tu conexión e intenta de nuevo.</p>}
        {/* Outlives the upload step: the preview replaces it at once. */}
        {(step === "upload" || step === "preview") && flow.notice !== undefined && <p role="status" style={{ ...noteStyle, fontWeight: 600 }}>{flow.notice}</p>}
        {step === "cycle" && <CycleStep cycle={cycle} today={today} />}
        {step === "upload" && cycle.state === "ready" && <UploadStep reading={flow.reading} error={flow.error} onFiles={(files) => void flow.chooseFiles(files)} />}
        {step === "preview" && flow.preview !== undefined && (
          <>
            {flow.error !== undefined && <p role="alert" style={alertStyle}>{flow.error}</p>}
            <PreviewStep result={flow.preview} levels={levels} onChoose={flow.choose} onCapacity={flow.setCapacity} onImport={() => void flow.commit()} onBack={flow.startOver} onCancel={onBack} onOpenTuition={onOpenTuition} focusUnresolved={flow.focusUnresolved} />
          </>
        )}
        {committing && <p role="status" style={{ fontSize: 16, fontWeight: 700, color: "var(--text)" }}>{busyText((flow.preview?.summary.new ?? 0) + (flow.preview?.summary.pending ?? 0), flow.preview?.summary.movedToCycleStart ?? 0)}</p>}
        {flow.phase === "result" && flow.result !== undefined && (
          <ResultStep result={flow.result} cut={flow.cut} onAnother={flow.startOver} onViewStudents={onViewStudents} />
        )}
      </section>
    </Page>
  );
}

/** The school's levels for the mapping chips, read once a preview needs them. */
function useSchoolLevels(needed: unknown): Level[] {
  const [levels, setLevels] = useState<Level[]>([]);
  const wanted = needed !== undefined;
  useEffect(() => {
    if (!wanted) return;
    let live = true;
    listLevels(API_URL).then((l) => live && setLevels(l)).catch(() => undefined);
    return () => {
      live = false;
    };
  }, [wanted]);
  return levels;
}

/** Moves focus to the step's heading each time the step changes (not on first paint). */
function useFocusOnChange(heading: { current: HTMLHeadingElement | null }, step: StepId | undefined): void {
  const first = useRef<StepId | undefined>(undefined);
  useEffect(() => {
    if (step === undefined) return;
    if (first.current === undefined) {
      first.current = step;
      return;
    }
    heading.current?.focus();
  }, [heading, step]);
}

/** While the commit runs, closing the tab asks first; never otherwise. */
function useWarnBeforeLeaving(active: boolean): void {
  useEffect(() => {
    if (!active) return;
    const warn = (e: BeforeUnloadEvent): void => e.preventDefault();
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [active]);
}
