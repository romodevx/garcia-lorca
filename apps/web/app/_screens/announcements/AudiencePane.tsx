"use client";

import { useId, type ReactElement, type ReactNode } from "react";
import type { AnnouncementScope } from "../../comms/api";
import { SCOPES } from "./audience";
import { TargetRow } from "./TargetRow";

export interface AudiencePaneProps {
  scope: AnnouncementScope;
  onChooseScope: (scope: AnnouncementScope) => void;
  pickedCount: number;
  /** What the chosen audience offers: groups by level, levels, the pupil search. */
  children: ReactNode;
}

/**
 * "Destinatarios", the left column of Avisos (PLAN-redesign-avisos §10): the
 * four audiences as radio-looking rows, the chosen audience's list under them
 * and the B6 counter at the foot.
 */
export function AudiencePane({ scope, onChooseScope, pickedCount, children }: AudiencePaneProps): ReactElement {
  const titleId = useId();
  return (
    <section className="announce-card announce-pane" aria-labelledby={titleId}>
      <h2 id={titleId} className="announce-pane__title">Destinatarios</h2>
      <div className="announce-scopes">
        {SCOPES.map((s) => (
          <TargetRow key={s.value} label={s.label} text={s.label} on={scope === s.value} onToggle={() => onChooseScope(s.value)} mark="radio" />
        ))}
      </div>
      <div className="announce-pane__list">{children}</div>
      {pickedCount > 1 && <p className="announce-pane__count">{pickedCount} seleccionados</p>}
    </section>
  );
}
