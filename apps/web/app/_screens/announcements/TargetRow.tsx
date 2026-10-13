"use client";

import type { ReactElement } from "react";
import { Icon } from "../../_design/Icon";

/** The ✓ inside a checked box, in px. */
const CHECK_SIZE = 14;
const CHECK_STROKE = 3;

export interface TargetRowProps {
  /** The accessible name: today's (B3), even when the row shows less. */
  label: string;
  /** What the row shows ("1° A" under its level). */
  text: string;
  /** A second line ("Primaria · 1° A" under a pupil); never part of the name. */
  detail?: string;
  on: boolean;
  onToggle: () => void;
  /** A radio dot for one-of (audiences, the pupil), a box for several. */
  mark: "radio" | "check";
}

/**
 * One row of the audience pane. It looks like a radio or a checkbox but stays
 * a `button` with `aria-pressed`, the control the characterization suite and
 * today's screen use (PLAN-redesign-avisos §10, D2).
 */
export function TargetRow({ label, text, detail, on, onToggle, mark }: TargetRowProps): ReactElement {
  return (
    <button type="button" className="announce-row" aria-label={label} aria-pressed={on} onClick={onToggle}>
      <span className={`announce-mark announce-mark--${mark}`} aria-hidden="true">
        {on && mark === "check" && <Icon name="check" size={CHECK_SIZE} stroke={CHECK_STROKE} className="announce-mark__check" />}
      </span>
      <span className="announce-row__text">
        {text}
        {detail !== undefined && <span className="announce-row__detail">{detail}</span>}
      </span>
    </button>
  );
}
