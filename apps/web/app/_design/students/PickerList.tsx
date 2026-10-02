"use client";

import type { DragEvent, KeyboardEvent, ReactElement, ReactNode } from "react";

import type { FilterableStudent } from "./studentFilter";
import { formatStudentName } from "./studentName";

/** Design system: a target is at least 44 px tall. */
const ROW_MIN_HEIGHT = 48;
const LIST_MAX_HEIGHT = 460;
const CHECKBOX_SIZE = 20;

export interface PickerListProps<T extends FilterableStudent> {
  items: readonly T[];
  selected: ReadonlySet<string>;
  onToggle: (id: string) => void;
  /** The row's id dragged; the picker decides what travels with it. */
  onDragStart: (id: string, e: DragEvent<HTMLLIElement>) => void;
  /** Under the name: the group, the level. */
  describe?: (item: T) => string;
  /** Beside the name, outside the checkbox's label (a select, a badge). */
  extra?: (item: T) => ReactNode;
  readOnly: boolean;
  empty: string;
}

/**
 * One side of the DualPanelPicker: each pupil a native checkbox (Tab reaches
 * it, Space toggles, and Enter too) that can also be dragged.
 */
export function PickerList<T extends FilterableStudent>({
  items, selected, onToggle, onDragStart, describe, extra, readOnly, empty,
}: PickerListProps<T>): ReactElement {
  if (items.length === 0) {
    return <p style={{ margin: 0, padding: "18px 14px", color: "var(--text-muted)", fontSize: 14.5 }}>{empty}</p>;
  }
  const enterToggles = (id: string) => (e: KeyboardEvent<HTMLInputElement>): void => {
    if (e.key !== "Enter") return;
    e.preventDefault();
    onToggle(id);
  };
  return (
    <ul style={{ listStyle: "none", margin: 0, padding: 0, maxHeight: LIST_MAX_HEIGHT, overflowY: "auto" }}>
      {items.map((item) => {
        const name = formatStudentName(item);
        const on = selected.has(item.id);
        const sub = describe?.(item);
        return (
          <li
            key={item.id}
            data-testid={`picker-row-${item.id}`}
            draggable={!readOnly}
            onDragStart={readOnly ? undefined : (e) => onDragStart(item.id, e)}
            style={{
              display: "flex", alignItems: "center", gap: 12, minHeight: ROW_MIN_HEIGHT, padding: "6px 14px",
              borderTop: "1px solid var(--border)", cursor: readOnly ? "default" : "grab",
              background: on ? "color-mix(in oklab, var(--lime) 16%, var(--surface))" : "var(--surface)",
            }}
          >
            <label style={{ display: "flex", alignItems: "center", gap: 12, flex: 1, minWidth: 0, cursor: readOnly ? "default" : "pointer" }}>
              {!readOnly && (
                <input
                  type="checkbox"
                  checked={on}
                  aria-label={name}
                  onChange={() => onToggle(item.id)}
                  onKeyDown={enterToggles(item.id)}
                  style={{ width: CHECKBOX_SIZE, height: CHECKBOX_SIZE, accentColor: "var(--forest)", flexShrink: 0 }}
                />
              )}
              <span style={{ minWidth: 0 }}>
                <span style={{ display: "block", fontSize: 15, fontWeight: 600, color: "var(--text)" }}>{name}</span>
                {sub !== undefined && sub !== "" && <span style={{ display: "block", fontSize: 12.5, color: "var(--text-faint)" }}>{sub}</span>}
              </span>
            </label>
            {extra?.(item)}
          </li>
        );
      })}
    </ul>
  );
}
