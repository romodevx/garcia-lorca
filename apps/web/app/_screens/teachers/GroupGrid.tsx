"use client";

import { useEffect, useId, useRef, type ReactElement } from "react";

import { isSelectable, type GradeChoices, type GroupChoice, type LevelChoices } from "../../teachers/load";

export interface GroupGridProps {
  tree: readonly LevelChoices[];
  selected: ReadonlySet<string>;
  onChange: (next: ReadonlySet<string>) => void;
}

/**
 * The groups of the cycle, level → grade → group, for ONE subject name. Each
 * level has a tri-state "Todos los de <Nivel>" that reaches only the groups
 * that can be picked; a group already this teacher's shows checked and locked.
 * Grades without the subject are not in the tree at all (teachers/load.ts).
 */
export function GroupGrid({ tree, selected, onChange }: GroupGridProps): ReactElement {
  const toggle = (ids: readonly string[], on: boolean): void => {
    const next = new Set(selected);
    for (const id of ids) {
      if (on) next.add(id);
      else next.delete(id);
    }
    onChange(next);
  };

  return (
    <div style={{ display: "grid", gap: 14 }}>
      {tree.map((level) => (
        <LevelBlock key={level.level.id} level={level} selected={selected} onToggle={toggle} />
      ))}
    </div>
  );
}

type Toggle = (ids: readonly string[], on: boolean) => void;

function LevelBlock({ level, selected, onToggle }: { level: LevelChoices; selected: ReadonlySet<string>; onToggle: Toggle }): ReactElement {
  const choices = level.grades.flatMap((g) => g.groups);
  const eligible = choices.filter((c) => isSelectable(c.availability)).map((c) => c.group.id);
  const picked = eligible.filter((id) => selected.has(id)).length;
  // Nothing left to pick because the teacher already holds it all: covered, not empty.
  const allMine = eligible.length === 0 && choices.some((c) => c.availability.kind === "mine");
  const all = allMine || (eligible.length > 0 && picked === eligible.length);
  const some = picked > 0 && !all;
  const allRef = useRef<HTMLInputElement>(null);

  // `indeterminate` exists only as a DOM property, never as an attribute.
  useEffect(() => {
    if (allRef.current !== null) allRef.current.indeterminate = some;
  }, [some]);

  return (
    <fieldset style={{ border: "1px solid var(--border)", borderRadius: "var(--r-lg)", padding: "10px 14px 14px", margin: 0, minWidth: 0 }}>
      <legend style={{ fontSize: 15, fontWeight: 700, color: "var(--text)", padding: "0 6px" }}>{level.level.name}</legend>
      <label style={{ ...rowStyle, fontWeight: 700 }}>
        <input
          ref={allRef}
          type="checkbox"
          checked={all}
          disabled={eligible.length === 0}
          onChange={() => onToggle(eligible, !all)}
          style={boxStyle}
        />
        Todos los de {level.level.name}
      </label>
      {level.grades.map((grade) => (
        <GradeRow key={grade.grade.id} grade={grade} selected={selected} onToggle={onToggle} />
      ))}
    </fieldset>
  );
}

function GradeRow({ grade, selected, onToggle }: { grade: GradeChoices; selected: ReadonlySet<string>; onToggle: Toggle }): ReactElement {
  return (
    <div style={{ marginTop: 10 }}>
      <div style={{ fontSize: 13, fontWeight: 700, color: "var(--text-muted)" }}>{grade.grade.name}</div>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginTop: 6 }}>
        {grade.groups.map((choice) => (
          <GroupBox key={choice.group.id} choice={choice} selected={selected} onToggle={onToggle} />
        ))}
      </div>
    </div>
  );
}

function GroupBox({ choice, selected, onToggle }: { choice: GroupChoice; selected: ReadonlySet<string>; onToggle: Toggle }): ReactElement {
  const noteId = useId();
  const { group, availability } = choice;
  const note = availability.kind === "mine" ? "ya asignado" : availability.kind === "held" ? `La imparte ${availability.teacherName}` : undefined;
  const describedBy = note !== undefined ? noteId : undefined;
  const locked = !isSelectable(availability);
  const checked = availability.kind === "mine" || selected.has(group.id);

  return (
    <label
      style={{ ...rowStyle, padding: "4px 14px 4px 10px", borderRadius: "var(--r-md)", border: `1.5px solid ${checked && !locked ? "var(--forest)" : "var(--border)"}`, background: "var(--surface)", cursor: locked ? "not-allowed" : "pointer" }}
    >
      <input
        type="checkbox"
        aria-label={group.label}
        aria-describedby={describedBy}
        checked={checked}
        disabled={locked}
        onChange={() => onToggle([group.id], !selected.has(group.id))}
        style={boxStyle}
      />
      <span style={{ display: "grid" }}>
        <span style={{ fontWeight: 700 }}>{group.name}</span>
        {note !== undefined && (
          <span id={noteId} style={{ fontSize: 12.5, color: "var(--text-muted)" }}>
            {note}
          </span>
        )}
      </span>
    </label>
  );
}

const rowStyle = { display: "inline-flex", alignItems: "center", gap: 10, minHeight: 44, fontSize: 14.5, color: "var(--text)" } as const;
const boxStyle = { width: 20, height: 20, accentColor: "var(--forest)", flexShrink: 0 } as const;
