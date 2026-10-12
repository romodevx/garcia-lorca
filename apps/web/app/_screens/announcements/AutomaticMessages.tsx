"use client";

import type { ReactElement } from "react";
import { Icon } from "../../_design/Icon";
import { BIRTHDAY_HEADING_ID, BirthdayGreeting } from "./BirthdayGreeting";

const STAR_SIZE = 18;
const CHEVRON_SIZE = 16;

/**
 * The automatic messages of Avisos, folded below the form (PLAN-redesign-avisos
 * §10, D13, D14): the summary is the control, and the birthday greeting is
 * mounted inside from the start, so it loads (A5) as it does today.
 */
export function AutomaticMessages(): ReactElement {
  return (
    <details className="announce-card announce-auto">
      <summary className="announce-auto__summary">
        <span className="announce-auto__icon" aria-hidden="true">
          <Icon name="star" size={STAR_SIZE} />
        </span>
        <h2 id={BIRTHDAY_HEADING_ID} className="announce-auto__title">Felicitación de cumpleaños</h2>
        <span className="announce-auto__meta">Mensaje automático · 7:00</span>
        <span className="announce-auto__edit" aria-hidden="true">
          Editar
          <Icon name="chevronDown" size={CHEVRON_SIZE} className="announce-auto__chevron" />
        </span>
      </summary>
      <div className="announce-auto__body">
        <BirthdayGreeting embedded />
      </div>
    </details>
  );
}
