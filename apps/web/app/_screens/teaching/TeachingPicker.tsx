"use client";

import { useEffect, useRef, type ReactElement, type ReactNode } from "react";

import { EmptyHint, Section } from "../config/parts";
import { TeachingCrumbs } from "./TeachingCrumbs";
import type { TeachingOption } from "./selection";
import { useTeachingSelection } from "./useTeachingSelection";

export interface TeachingPickerProps<T extends TeachingOption> {
  /** The module, first crumb: "Calificaciones", "Asistencia". */
  title: string;
  options: readonly T[];
  /** Subject first, then group (grades); otherwise group only (attendance). */
  bySubject: boolean;
  /** The list for the chosen class or group. */
  children: (selected: T) => ReactNode;
}

const EMPTY = "Aún no tienes grupos asignados. Pide a la dirección que te asigne tu carga académica.";

/**
 * How a teacher reaches a list: subject → group → the list, skipping any step
 * that has a single choice, with the place kept in the URL (Back works,
 * reload stays) and a breadcrumb to step back. Large tappable choices, not
 * selects: this is used standing, on a phone, between classes.
 */
export function TeachingPicker<T extends TeachingOption>({ title, options, bySubject, children }: TeachingPickerProps<T>): ReactElement {
  const selection = useTeachingSelection(options, bySubject);
  const heading = useRef<HTMLHeadingElement>(null);
  const previousStep = useRef(selection.step);

  // A new step moves the focus to its question, so keyboard and screen-reader
  // users land where the choices are. Not on first paint: that would steal it.
  useEffect(() => {
    if (previousStep.current !== selection.step) heading.current?.focus();
    previousStep.current = selection.step;
  }, [selection.step]);

  if (selection.step === "empty") {
    return (
      <Section title={title}>
        <EmptyHint>{EMPTY}</EmptyHint>
      </Section>
    );
  }

  return (
    <div>
      <TeachingCrumbs title={title} selection={selection} options={options} bySubject={bySubject} />
      {selection.step === "subject" && (
        <ChoiceStep heading={heading} title="Elige la asignatura" listLabel="Asignaturas">
          {selection.subjects.map((s) => {
            const count = options.filter((o) => o.subjectName === s).length;
            return (
              <ChoiceButton key={s} onClick={() => selection.chooseSubject(s)} detail={`${count} ${count === 1 ? "grupo" : "grupos"}`}>
                {s}
              </ChoiceButton>
            );
          })}
        </ChoiceStep>
      )}
      {selection.step === "group" && (
        <ChoiceStep heading={heading} title="Elige el grupo" listLabel="Grupos">
          {selection.groups.map((g) => (
            <ChoiceButton key={g.id} onClick={() => selection.chooseGroup(g.groupId)}>{g.groupName}</ChoiceButton>
          ))}
        </ChoiceStep>
      )}
      {selection.step === "ready" && selection.selected !== undefined && children(selection.selected)}
    </div>
  );
}

function ChoiceStep({ heading, title, listLabel, children }: {
  heading: React.RefObject<HTMLHeadingElement | null>;
  title: string;
  listLabel: string;
  children: ReactNode;
}): ReactElement {
  return (
    <section>
      <h2 ref={heading} tabIndex={-1} style={{ fontSize: 18, fontWeight: 800, color: "var(--text)", margin: "0 0 12px", outline: "none" }}>{title}</h2>
      <ul aria-label={listLabel} style={{ listStyle: "none", margin: 0, padding: 0, display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(140px, 1fr))", gap: 10 }}>
        {children}
      </ul>
    </section>
  );
}

function ChoiceButton({ onClick, detail, children }: { onClick: () => void; detail?: string; children: ReactNode }): ReactElement {
  return (
    <li>
      <button
        type="button"
        onClick={onClick}
        style={{ width: "100%", minHeight: 72, padding: "12px 16px", display: "grid", alignContent: "center", gap: 2, textAlign: "left", borderRadius: "var(--r-lg)", background: "var(--surface)", border: "1.5px solid var(--border)", boxShadow: "var(--shadow-card)", color: "var(--text)", fontFamily: "var(--font-display)", fontSize: 17, fontWeight: 700, cursor: "pointer" }}
      >
        {children}
        {detail !== undefined && <span style={{ fontFamily: "var(--font-ui)", fontSize: 13, fontWeight: 500, color: "var(--text-muted)" }}>{detail}</span>}
      </button>
    </li>
  );
}
