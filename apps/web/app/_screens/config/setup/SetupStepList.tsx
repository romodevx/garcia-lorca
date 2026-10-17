"use client";

import { useState, type CSSProperties, type ReactElement } from "react";

import { Icon } from "../../../_design/Icon";
import { SetupStepBar } from "./SetupStepBar";
import { PENDING_TEXT, QUIET_TEXT } from "./setupColors";
import type { StepId, StepStatus, StepView } from "./setupSteps";

/*
 * The "Puesta en marcha" card (PLAN-redesign-configuracion §1, §1.5–§1.7): the
 * eight steps with their status, and Herramientas under a rule. Below the narrow
 * breakpoint it folds into a bar naming the open step (SetupStepBar).
 */

/** What the right column shows: a step, or the Papelera. */
export type OpenTarget = StepId | "trash";

export interface SetupStepListProps {
  steps: readonly StepView[];
  cardTitle: string;
  /** Undefined until the opening step is decided (§1.4). */
  openId: OpenTarget | undefined;
  onOpen: (id: OpenTarget) => void;
  /** Migrar alumnos; given only when the caller may import (A2/A12). */
  onOpenStudentImport?: () => void;
  /** What the import will create of the catalog; undefined says nothing (ISSUE-024). */
  importHint?: string;
  /** The Papelera; only with levels:delete. */
  showTrash: boolean;
  narrow: boolean;
}

export function SetupStepList(props: SetupStepListProps): ReactElement {
  const armed = useArmedChecks(props.steps);
  if (props.narrow) return <SetupStepBar {...props} list={<StepList {...props} armed={armed} />} />;
  return (
    <aside className="setup-list" style={cardStyle}>
      <StepList {...props} armed={armed} />
    </aside>
  );
}

/**
 * The steps whose ✓ may animate in: those seen pending (or unread) in this
 * visit. A step already done when its status first arrives just shows its ✓ —
 * the scale-in marks finishing a step, not loading the page (§1.7).
 */
function useArmedChecks(steps: readonly StepView[]): ReadonlySet<StepId> {
  const [armed, setArmed] = useState<ReadonlySet<StepId>>(() => new Set());
  const waiting = steps.filter((s) => (s.status === "pending" || s.status === "unknown") && !armed.has(s.id));
  // Adjusting state while rendering, from the previous render's state (React's documented pattern).
  if (waiting.length > 0) setArmed(new Set([...armed, ...waiting.map((s) => s.id)]));
  return armed;
}

const cardStyle: CSSProperties = {
  background: "var(--surface)",
  border: "1px solid var(--border)",
  borderRadius: "var(--r-xl)",
  boxShadow: "var(--shadow-card)",
  padding: "18px 12px",
  minWidth: 0,
};

const cardTitleStyle: CSSProperties = {
  padding: "0 10px 10px",
  fontSize: 12.5,
  fontWeight: 700,
  letterSpacing: ".06em",
  textTransform: "uppercase",
  color: "var(--text-muted)",
};

interface StepListProps extends SetupStepListProps {
  armed: ReadonlySet<StepId>;
}

function StepList({ steps, cardTitle, openId, onOpen, onOpenStudentImport, importHint, showTrash, armed }: StepListProps): ReactElement {
  return (
    <>
      <nav aria-label={cardTitle}>
        <p aria-hidden="true" style={cardTitleStyle}>{cardTitle}</p>
        <ol style={{ listStyle: "none", margin: 0, padding: 0, display: "grid", gap: 2 }}>
          {steps.map((s) => (
            <li key={s.id}>
              <StepRow step={s} open={s.id === openId} animateCheck={armed.has(s.id)} onOpen={() => onOpen(s.id)} />
            </li>
          ))}
        </ol>
      </nav>
      <Tools importHint={importHint} onOpenStudentImport={onOpenStudentImport} showTrash={showTrash} trashOpen={openId === "trash"} onOpenTrash={() => onOpen("trash")} />
    </>
  );
}

const rowStyle = (open: boolean): CSSProperties => ({
  display: "flex",
  alignItems: "center",
  gap: 12,
  width: "100%",
  minHeight: 56,
  padding: "9px 10px",
  borderRadius: 14,
  textAlign: "left",
  cursor: "pointer",
  background: open ? "color-mix(in oklab, var(--lime) 18%, var(--surface))" : "transparent",
});

/** The sub-line's colour: what is missing reads as a warning, the rest is quiet. */
const LINE_COLOR: Partial<Record<StepStatus, string>> = { pending: PENDING_TEXT };

function StepRow({ step, open, animateCheck, onOpen }: { step: StepView; open: boolean; animateCheck: boolean; onOpen: () => void }): ReactElement {
  const full = step.line === undefined ? step.title : `${step.title} · ${step.line}`;
  return (
    <button type="button" onClick={onOpen} aria-current={open ? "step" : undefined} title={full} style={rowStyle(open)}>
      <StepMark step={step} open={open} animateCheck={animateCheck} />
      <span style={{ minWidth: 0, display: "grid" }}>
        <span style={{ fontSize: 15, fontWeight: 700, color: "var(--text)" }}>{step.title}</span>
        {step.line !== undefined && (
          <span data-line className="setup-step-line" style={{ fontSize: 13, color: LINE_COLOR[step.status] ?? "var(--text-muted)" }}>
            {step.line}
          </span>
        )}
      </span>
    </button>
  );
}

const MARK_SIZE = 30;

function markStyle(status: StepStatus, open: boolean): CSSProperties {
  const done = status === "done";
  const base: CSSProperties = { width: MARK_SIZE, height: MARK_SIZE, flexShrink: 0, borderRadius: "50%", display: "grid", placeItems: "center", fontWeight: 800, fontSize: 13.5, borderWidth: 2, borderStyle: "solid", borderColor: "var(--border-strong)", background: "var(--surface)", color: "var(--text-muted)" };
  if (open) return { ...base, borderColor: "var(--lime)", background: "var(--lime)", color: "var(--primary-ink)" };
  if (done) return { ...base, borderColor: "var(--forest)", background: "var(--forest)", color: "#fff" };
  if (status === "loading") return { ...base, color: "var(--text-faint)", borderColor: "var(--border)" };
  return base;
}

function StepMark({ step, open, animateCheck }: { step: StepView; open: boolean; animateCheck: boolean }): ReactElement {
  return (
    <span aria-hidden="true" data-mark={step.status} style={markStyle(step.status, open)}>
      {step.status === "done" ? (
        <Icon name="check" size={16} stroke={3} className={animateCheck ? "setup-check-in" : undefined} />
      ) : (
        step.number
      )}
    </span>
  );
}

const toolStyle: CSSProperties = { display: "flex", alignItems: "center", gap: 10, width: "100%", minHeight: 48, padding: "8px 10px", borderRadius: 12, textAlign: "left", color: "var(--text-muted)", fontWeight: 600, fontSize: 14.5, cursor: "pointer" };

interface ToolsProps {
  importHint: string | undefined;
  onOpenStudentImport?: () => void;
  showTrash: boolean;
  trashOpen: boolean;
  onOpenTrash: () => void;
}

function Tools({ importHint, onOpenStudentImport, showTrash, trashOpen, onOpenTrash }: ToolsProps): ReactElement | null {
  if (onOpenStudentImport === undefined && !showTrash) return null;
  return (
    <div style={{ borderTop: "1px solid var(--border)", margin: "12px 10px 0", paddingTop: 10 }}>
      <p style={{ ...cardTitleStyle, padding: "0 0 6px" }}>Herramientas</p>
      {onOpenStudentImport !== undefined && (
        <button type="button" onClick={onOpenStudentImport} style={toolStyle}>
          <Icon name="upload" size={18} />
          <span style={{ display: "grid" }}>
            Migrar alumnos del sistema anterior
            {importHint !== undefined && <span style={{ fontSize: 13, fontWeight: 500, color: QUIET_TEXT }}>{importHint}</span>}
          </span>
        </button>
      )}
      {showTrash && (
        <button type="button" onClick={onOpenTrash} aria-current={trashOpen ? "true" : undefined} style={{ ...toolStyle, background: trashOpen ? "color-mix(in oklab, var(--lime) 18%, var(--surface))" : "transparent" }}>
          <Icon name="trash" size={18} />
          Papelera
        </button>
      )}
    </div>
  );
}
