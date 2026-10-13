"use client";

import { useState, type ReactElement } from "react";
import { Icon } from "../../_design/Icon";
import type { Target, TargetSection } from "./audience";
import { TargetRow } from "./TargetRow";

/** Up to this many levels, every one starts open; above it, only the ones with a pick. */
const OPEN_ALL_MAX = 3;
const CHEVRON_SIZE = 16;

export interface GroupedTargetsProps {
  sections: TargetSection[];
  picked: string[];
  onToggle: (id: string) => void;
  /** False until the list's own load settles (D11). */
  loaded: boolean;
  /** What to say when the list arrived empty (or failed: A2–A4 are silent). */
  emptyText: string;
}

/**
 * The groups of Avisos filed by level, or the levels as one plain list
 * (PLAN-redesign-avisos §10). Several can be picked (B4); nothing is cut off,
 * the list grows with the school.
 */
export function GroupedTargets({ sections, picked, onToggle, loaded, emptyText }: GroupedTargetsProps): ReactElement {
  if (!loaded) return <p className="announce-note">Cargando…</p>;
  if (sections.length === 0) return <p className="announce-note">{emptyText}</p>;
  const [only] = sections;
  if (sections.length === 1 && only?.title === undefined) {
    return <TargetRows targets={only?.targets ?? []} picked={picked} onToggle={onToggle} />;
  }
  return (
    <div className="announce-levels">
      {sections.map((s) => (
        <LevelSection key={s.key} section={s} picked={picked} onToggle={onToggle} openByDefault={sections.length <= OPEN_ALL_MAX} />
      ))}
    </div>
  );
}

interface LevelSectionProps {
  section: TargetSection;
  picked: string[];
  onToggle: (id: string) => void;
  openByDefault: boolean;
}

function LevelSection({ section, picked, onToggle, openByDefault }: LevelSectionProps): ReactElement {
  const total = section.targets.length;
  const chosen = section.targets.filter((t) => picked.includes(t.id)).length;
  // Decided once, when the level appears: after that the office opens and
  // closes it, and a re-render never snaps it back.
  const [open, setOpen] = useState(openByDefault || chosen > 0);
  return (
    <details className="announce-level" open={open} onToggle={(e) => setOpen(e.currentTarget.open)}>
      <summary className="announce-level__summary">
        <Icon name="chevron" size={CHEVRON_SIZE} className="announce-level__chevron" />
        <span className="announce-level__title">{section.title}</span>
        <span className="announce-level__count">{chosen > 0 ? `${chosen} de ${total}` : total}</span>
      </summary>
      <TargetRows targets={section.targets} picked={picked} onToggle={onToggle} />
    </details>
  );
}

interface TargetRowsProps {
  targets: Target[];
  picked: string[];
  onToggle: (id: string) => void;
}

function TargetRows({ targets, picked, onToggle }: TargetRowsProps): ReactElement {
  return (
    <div className="announce-rows">
      {targets.map((t) => (
        <TargetRow key={t.id} label={t.label} text={t.text} on={picked.includes(t.id)} onToggle={() => onToggle(t.id)} mark="check" />
      ))}
    </div>
  );
}
