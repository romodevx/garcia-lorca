"use client";

import type { CSSProperties, ReactElement, ReactNode } from "react";

import { Btn } from "../../../_design/Controls";
import { UnderStepTitle } from "./SetupPresentation";
import { STEPS, type StepDef, type StepId, type StepView } from "./setupSteps";

/*
 * The open step's frame (PLAN-redesign-configuracion §1.2–§1.3): "Paso N de 8",
 * the body, and the footer back/next. It has no title or purpose line of its
 * own — the panel's h2 and subtitle are those — unless the body has no panel
 * yet, or chips sit above the panel: then the frame draws the h2 and the
 * panel's title drops to a sub-heading, so the title never moves.
 */

/** What every step receives to sit in the right column. */
export interface StepSlot {
  step: StepView;
  hidden: boolean;
  onGo: (id: StepId) => void;
}

export interface StepFrameProps {
  /** The step framed; absent for a tool (the Papelera): no step label, no footer. */
  step?: StepView;
  tool?: boolean;
  /** Not the open step: hidden, never unmounted (§3.1). */
  hidden?: boolean;
  /** False while prices take the screen: only the body is drawn. */
  chrome?: boolean;
  /** The body has no panel h2 (a prerequisite or a pick is missing): title the step here. */
  ownHeading?: boolean;
  onGo: (id: StepId) => void;
  children: ReactNode;
}

const frameStyle: CSSProperties = {
  background: "var(--surface)",
  border: "1px solid var(--border)",
  borderRadius: "var(--r-xl)",
  boxShadow: "var(--shadow-card)",
  minWidth: 0,
};

const bodyStyle: CSSProperties = { padding: "26px 28px", minWidth: 0 };

const eyebrowStyle: CSSProperties = {
  fontSize: 12.5,
  fontWeight: 700,
  letterSpacing: ".06em",
  textTransform: "uppercase",
  color: "var(--text-faint)",
};

const footStyle: CSSProperties = {
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  justifyContent: "space-between",
  gap: 12,
  padding: "14px 28px",
  borderTop: "1px solid var(--border)",
  background: "var(--surface-2)",
  borderRadius: "0 0 var(--r-xl) var(--r-xl)",
};

const textLinkStyle: CSSProperties = { minHeight: 44, color: "var(--text-muted)", fontWeight: 600, fontSize: 14.5, cursor: "pointer" };

const neighbour = (step: StepDef, offset: number): StepDef | undefined => STEPS[step.number - 1 + offset];

export function StepFrame({ step, tool = false, hidden = false, chrome = true, ownHeading = false, onGo, children }: StepFrameProps): ReactElement {
  const framed = chrome && !tool ? step : undefined;
  return (
    <section hidden={hidden} data-step={tool ? "trash" : step?.id} style={chrome ? frameStyle : undefined}>
      {/* The body keeps the same place in the tree with or without chrome (§3.3). */}
      <div className={chrome ? "setup-step-body" : undefined} style={chrome ? bodyStyle : undefined}>
        {framed !== undefined && <StepIntro step={framed} ownHeading={ownHeading} />}
        {framed !== undefined && ownHeading ? <UnderStepTitle title={framed.title}>{children}</UnderStepTitle> : children}
      </div>
      {framed !== undefined && <StepFoot step={framed} onGo={onGo} />}
    </section>
  );
}

function StepIntro({ step, ownHeading }: { step: StepView; ownHeading: boolean }): ReactElement {
  return (
    <div style={{ marginBottom: 18 }}>
      <p style={eyebrowStyle}>Paso {step.number} de {STEPS.length}</p>
      {/* With a panel, its own h2 and subtitle already title the step and say what it is for. */}
      {ownHeading && (
        <>
          <h2 tabIndex={-1} style={{ fontSize: 18, fontWeight: 700, color: "var(--text)", marginTop: 4 }}>
            {step.title}
          </h2>
          <p style={{ fontSize: 14.5, color: "var(--text-muted)", marginTop: 4 }}>{step.purpose}</p>
        </>
      )}
    </div>
  );
}

function StepFoot({ step, onGo }: { step: StepView; onGo: (id: StepId) => void }): ReactElement {
  const previous = neighbour(step, -1);
  const next = neighbour(step, 1);
  return (
    <div style={footStyle}>
      {previous !== undefined ? (
        <button type="button" onClick={() => onGo(previous.id)} aria-label={`Anterior: ${previous.title}`} style={textLinkStyle}>
          ← {previous.title}
        </button>
      ) : (
        <span />
      )}
      {next !== undefined && step.next !== "none" && <NextButton next={next} lime={step.next === "primary"} onGo={onGo} />}
    </div>
  );
}

function NextButton({ next, lime, onGo }: { next: StepDef; lime: boolean; onGo: (id: StepId) => void }): ReactElement {
  const label = `Siguiente: ${next.title}`;
  if (lime) {
    return <Btn onClick={() => onGo(next.id)} aria-label={label} style={{ height: 44 }}>{label} →</Btn>;
  }
  return (
    <button type="button" onClick={() => onGo(next.id)} aria-label={label} style={{ ...textLinkStyle, color: "var(--forest)" }}>
      {label} →
    </button>
  );
}
