"use client";

import { useId, useRef, useState, type CSSProperties, type KeyboardEvent, type MouseEvent, type ReactElement, type ReactNode } from "react";

import { Icon } from "../../../_design/Icon";
import { STEPS } from "./setupSteps";
import type { OpenTarget, SetupStepListProps } from "./SetupStepList";

/*
 * Below the narrow breakpoint the step list folds into a sticky bar that names
 * the open step (PLAN §1.6): "Paso 6 de 8 · Grupos ▾". It is a disclosure — a
 * <button aria-expanded> — and Escape closes it and hands focus back.
 */

interface SetupStepBarProps extends SetupStepListProps {
  /** The full list, drawn under the bar while it is open. */
  list: ReactNode;
}

const barStyle: CSSProperties = {
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: 10,
  width: "100%",
  minHeight: 48,
  padding: "0 16px",
  borderRadius: "var(--r-lg)",
  background: "var(--surface)",
  border: "1px solid var(--border)",
  boxShadow: "var(--shadow-card)",
  fontSize: 15,
  fontWeight: 700,
  color: "var(--text)",
  cursor: "pointer",
};

const panelStyle: CSSProperties = {
  marginTop: 8,
  padding: "14px 10px",
  borderRadius: "var(--r-lg)",
  background: "var(--surface)",
  border: "1px solid var(--border)",
  boxShadow: "var(--shadow-pop)",
};

function barLabel(openId: OpenTarget | undefined, cardTitle: string): string {
  if (openId === "trash") return "Papelera";
  const step = STEPS.find((s) => s.id === openId);
  return step === undefined ? cardTitle : `Paso ${step.number} de ${STEPS.length} · ${step.title}`;
}

export function SetupStepBar({ openId, cardTitle, list }: SetupStepBarProps): ReactElement {
  const [expanded, setExpanded] = useState(false);
  const toggle = useRef<HTMLButtonElement>(null);
  const panelId = useId();

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>): void => {
    if (event.key !== "Escape" || !expanded) return;
    setExpanded(false);
    toggle.current?.focus();
  };

  // Choosing a row closes the list; the open step then takes focus (§1.7).
  const onClickCapture = (event: MouseEvent<HTMLDivElement>): void => {
    if (event.target instanceof Element && event.target.closest("button") !== null) setExpanded(false);
  };

  return (
    <div className="setup-mobilebar" onKeyDown={onKeyDown}>
      <button ref={toggle} type="button" aria-expanded={expanded} aria-controls={panelId} onClick={() => setExpanded((v) => !v)} style={barStyle}>
        {barLabel(openId, cardTitle)}
        <Icon name="chevron" size={18} style={{ transform: expanded ? "rotate(-90deg)" : "rotate(90deg)" }} />
      </button>
      {expanded && (
        <div id={panelId} className="setup-mobilebar-panel" style={panelStyle} onClickCapture={onClickCapture}>
          {list}
        </div>
      )}
    </div>
  );
}
