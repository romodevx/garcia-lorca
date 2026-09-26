"use client";

import { Fragment, useEffect, useRef, useState, type ReactElement } from "react";

import { subjectLabel, type TeachingOption } from "./selection";
import type { PickerStep, TeachingSelection } from "./useTeachingSelection";

interface Crumb {
  label: string;
  /** Where pressing it goes; undefined when there is nowhere to go (a skipped step). */
  target: PickerStep | undefined;
}

/** The first step the teacher actually sees; undefined when every step was skipped. */
function firstStep<T extends TeachingOption>(sel: TeachingSelection<T>, bySubject: boolean): PickerStep | undefined {
  if (bySubject && !sel.subjectSkipped) return "subject";
  return sel.groupSkipped ? undefined : "group";
}

function crumbsOf<T extends TeachingOption>(title: string, sel: TeachingSelection<T>, bySubject: boolean): Crumb[] {
  const crumbs: Crumb[] = [{ label: title, target: firstStep(sel, bySubject) }];
  if (bySubject && sel.subject !== undefined) crumbs.push({ label: sel.subject.name, target: sel.groupSkipped ? undefined : "group" });
  if (sel.selected !== undefined) crumbs.push({ label: sel.selected.groupName, target: undefined });
  return crumbs;
}

const CRUMB_BUTTON = { minHeight: 44, padding: "0 6px", color: "var(--forest)", fontWeight: 700, fontSize: 15, textDecoration: "underline", textUnderlineOffset: 3, cursor: "pointer" } as const;

export interface TeachingCrumbsProps<T extends TeachingOption> {
  title: string;
  selection: TeachingSelection<T>;
  options: readonly T[];
  bySubject: boolean;
}

/** "Calificaciones › Matemáticas › 2° B": each crumb returns to its step; "Cambiar" jumps anywhere. */
export function TeachingCrumbs<T extends TeachingOption>({ title, selection, options, bySubject }: TeachingCrumbsProps<T>): ReactElement {
  const crumbs = crumbsOf(title, selection, bySubject);
  return (
    <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: 8, marginBottom: 16 }}>
      <nav aria-label="Ruta">
        <ol style={{ display: "flex", flexWrap: "wrap", alignItems: "center", listStyle: "none", margin: 0, padding: 0, fontSize: 15 }}>
          {crumbs.map((c, i) => {
            const last = i === crumbs.length - 1;
            return (
              <Fragment key={`${i}-${c.label}`}>
                {i > 0 && <li aria-hidden="true" style={{ color: "var(--text-faint)", padding: "0 4px" }}>›</li>}
                <li>
                  {c.target !== undefined && !last ? (
                    <button type="button" onClick={() => selection.goTo(c.target as PickerStep)} style={CRUMB_BUTTON}>{c.label}</button>
                  ) : (
                    <span aria-current={last ? "page" : undefined} style={{ padding: "0 6px", fontWeight: last ? 800 : 600, color: last ? "var(--text)" : "var(--text-muted)" }}>{c.label}</span>
                  )}
                </li>
              </Fragment>
            );
          })}
        </ol>
      </nav>
      {selection.step === "ready" && options.length > 1 && (
        <SwitchMenu options={options} current={selection.selected} bySubject={bySubject} onChoose={selection.choose} />
      )}
    </div>
  );
}

function SwitchMenu<T extends TeachingOption>({ options, current, bySubject, onChoose }: {
  options: readonly T[];
  current: T | undefined;
  bySubject: boolean;
  onChoose: (option: T) => void;
}): ReactElement {
  const [open, setOpen] = useState(false);
  const toggle = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    panel.current?.querySelector("button")?.focus();
  }, [open]);

  const close = (): void => {
    setOpen(false);
    toggle.current?.focus();
  };

  return (
    <div style={{ position: "relative" }} onKeyDown={(e) => { if (e.key === "Escape" && open) close(); }}>
      <button
        ref={toggle}
        type="button"
        aria-expanded={open}
        aria-controls="teaching-switch-menu"
        onClick={() => setOpen((v) => !v)}
        style={{ minHeight: 44, padding: "0 16px", borderRadius: "var(--r-pill)", border: "1.5px solid var(--border)", background: "var(--surface)", color: "var(--text)", fontWeight: 700, fontSize: 14, cursor: "pointer" }}
      >
        Cambiar
      </button>
      {open && (
        <div id="teaching-switch-menu" ref={panel} style={{ position: "absolute", right: 0, top: "calc(100% + 6px)", zIndex: 20, width: "min(300px, calc(100vw - 32px))", maxHeight: 320, overflowY: "auto", padding: 6, borderRadius: "var(--r-md)", background: "var(--surface)", border: "1px solid var(--border)", boxShadow: "var(--shadow-pop)" }}>
          <ul style={{ listStyle: "none", margin: 0, padding: 0, display: "grid", gap: 2 }}>
            {options.map((o) => (
              <li key={o.id}>
                <button
                  type="button"
                  aria-current={o.id === current?.id ? "true" : undefined}
                  onClick={() => { onChoose(o); close(); }}
                  style={{ width: "100%", minHeight: 44, padding: "0 12px", textAlign: "left", borderRadius: 10, fontSize: 14.5, fontWeight: o.id === current?.id ? 800 : 600, color: "var(--text)", background: o.id === current?.id ? "var(--surface-2)" : "transparent", cursor: "pointer" }}
                >
                  {bySubject ? `${subjectLabel(o)} · ${o.groupName}` : o.groupName}
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
