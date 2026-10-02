"use client";

import { useEffect, useMemo, useRef, useState, type CSSProperties, type DragEvent, type ReactElement, type ReactNode } from "react";

import { Icon } from "../Icon";
import { PickerList } from "./PickerList";
import { StudentFilters } from "./StudentFilters";
import { compareStudents } from "./studentName";
import type { FilterCatalog, FilterableStudent } from "./studentFilter";
import { useStudentFilter } from "./useStudentFilter";

type Side = "available" | "assigned";

export interface DualPanelPickerLabels {
  /** The left panel: whoever is not assigned yet ("Alumnos"). */
  available: string;
  /** The right panel ("Inscritos", "Con beca"). */
  assigned: string;
  /** The → arrow ("Inscribir"). */
  assign: string;
  /** The ← arrow ("Dar de baja"). */
  unassign: string;
  emptyAssigned?: string;
}

export interface DualPanelPickerProps<T extends FilterableStudent> {
  /** Every pupil that could be assigned, placed (level, grade, group). */
  students: readonly T[];
  catalog: FilterCatalog;
  /** Who is assigned to the target now. */
  assignedIds: readonly string[];
  onAssign: (ids: string[]) => void;
  onUnassign: (ids: string[]) => void;
  labels: DualPanelPickerLabels;
  /** Under each name (the group). */
  describe?: (student: T) => string;
  /** Per assigned pupil, beside the name: the target's own choice (a price option). */
  renderAssignedExtra?: (student: T) => ReactNode;
  /** Above the right panel: the batch's choice for whoever moves in next. */
  assignedToolbar?: ReactNode;
  /** Without the write permission: both lists, nothing moves. */
  readOnly?: boolean;
}

const NOTHING_MARKED = "Marca primero a los alumnos que quieres mover.";

const panelStyle: CSSProperties = {
  flex: "1 1 340px", minWidth: 0, display: "grid", alignContent: "start", gap: 12, padding: 16,
  borderRadius: "var(--r-xl)", background: "var(--surface)", border: "1px solid var(--border)", boxShadow: "var(--shadow-card)",
};

const arrowStyle: CSSProperties = {
  minWidth: 48, minHeight: 48, padding: "0 14px", borderRadius: 99, display: "inline-flex", alignItems: "center", justifyContent: "center", gap: 6,
  fontWeight: 700, fontFamily: "var(--font-display)", fontSize: 14, cursor: "pointer", border: "1px solid var(--border)",
};

/**
 * Two lists and the arrows between them (plan A5): who is not assigned, with
 * the T4 filters, and who is. Several move at once — checked and sent with an
 * arrow, or dragged together — and both lists read surname first. Generic over
 * "assigned to a target": activities (T17) and scholarships (T18) share it.
 */
export function DualPanelPicker<T extends FilterableStudent>(props: DualPanelPickerProps<T>): ReactElement {
  const { students, catalog, assignedIds, onAssign, onUnassign, labels, describe, renderAssignedExtra, assignedToolbar, readOnly = false } = props;
  const assignedSet = useMemo(() => new Set(assignedIds), [assignedIds]);
  const available = useMemo(() => students.filter((s) => !assignedSet.has(s.id)), [students, assignedSet]);
  const assigned = useMemo(() => students.filter((s) => assignedSet.has(s.id)).sort(compareStudents), [students, assignedSet]);
  const filter = useStudentFilter(available, catalog);
  const [rightSelected, setRightSelected] = useState<ReadonlySet<string>>(() => new Set());
  const [hint, setHint] = useState("");
  const dragged = useRef<{ from: Side; ids: string[] } | null>(null);

  // Whoever leaves the right panel leaves its selection.
  useEffect(() => {
    setRightSelected((prev) => {
      const kept = [...prev].filter((id) => assignedSet.has(id));
      return kept.length === prev.size ? prev : new Set(kept);
    });
  }, [assignedSet]);

  const rightIds = (): string[] => assigned.filter((s) => rightSelected.has(s.id)).map((s) => s.id);
  const leftIds = (): string[] => filter.selectedItems.map((s) => s.id);

  const move = (from: Side, ids: string[]): void => {
    if (ids.length === 0) {
      setHint(NOTHING_MARKED);
      return;
    }
    setHint("");
    if (from === "available") {
      filter.clearSelection();
      onAssign(ids);
    } else {
      setRightSelected(new Set());
      onUnassign(ids);
    }
  };

  const toggleRight = (id: string): void => {
    setRightSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const startDrag = (from: Side) => (id: string, e: DragEvent<HTMLLIElement>): void => {
    const marked = from === "available" ? leftIds() : rightIds();
    const ids = marked.includes(id) ? marked : [id];
    dragged.current = { from, ids };
    e.dataTransfer.effectAllowed = "move";
    e.dataTransfer.setData("text/plain", ids.join(","));
  };

  const dropZone = (to: Side) => ({
    onDragOver: (e: DragEvent<HTMLElement>): void => {
      if (dragged.current !== null && dragged.current.from !== to) e.preventDefault();
    },
    onDrop: (e: DragEvent<HTMLElement>): void => {
      e.preventDefault();
      const drag = dragged.current;
      dragged.current = null;
      if (drag === null || drag.from === to) return;
      move(drag.from, drag.ids);
    },
  });

  const allRight = assigned.length > 0 && rightSelected.size === assigned.length;

  return (
    <div style={{ display: "grid", gap: 10 }}>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 16, alignItems: "stretch" }}>
        <section aria-label={labels.available} style={panelStyle} {...dropZone("available")}>
          <PanelTitle text={labels.available} count={available.length} />
          {!readOnly && <StudentFilters filter={filter} selectable />}
          <PickerList items={filter.filtered} selected={filter.selected} onToggle={filter.toggle} onDragStart={startDrag("available")}
            describe={describe} readOnly={readOnly} empty="No hay alumnos con ese filtro." />
        </section>

        {!readOnly && (
          <div style={{ display: "flex", flexDirection: "column", justifyContent: "center", gap: 10, flex: "0 0 auto" }}>
            <button type="button" onClick={() => move("available", leftIds())} style={{ ...arrowStyle, background: "var(--primary)", color: "var(--primary-ink)", borderColor: "var(--lime)" }}>
              {labels.assign}<Icon name="chevron" size={18} stroke={2.6} />
            </button>
            <button type="button" onClick={() => move("assigned", rightIds())} style={{ ...arrowStyle, background: "var(--surface)", color: "var(--text)" }}>
              <Icon name="back" size={18} stroke={2.6} />{labels.unassign}
            </button>
          </div>
        )}

        <section aria-label={labels.assigned} style={panelStyle} {...dropZone("assigned")}>
          <PanelTitle text={labels.assigned} count={assigned.length} />
          {assignedToolbar}
          {!readOnly && assigned.length > 0 && (
            <div>
              <button type="button" aria-pressed={allRight} onClick={() => setRightSelected(allRight ? new Set() : new Set(assigned.map((s) => s.id)))}
                style={{ ...arrowStyle, fontWeight: 600, background: allRight ? "color-mix(in oklab, var(--lime) 18%, var(--surface))" : "var(--surface)", color: "var(--text-muted)" }}>
                {`Seleccionar todos (${assigned.length})`}
              </button>
            </div>
          )}
          <PickerList items={assigned} selected={rightSelected} onToggle={toggleRight} onDragStart={startDrag("assigned")}
            describe={describe} extra={renderAssignedExtra} readOnly={readOnly} empty={labels.emptyAssigned ?? "Nadie todavía."} />
        </section>
      </div>
      <p role="status" style={{ margin: 0, minHeight: 20, fontSize: 14, color: "var(--warn-fg)" }}>{hint}</p>
    </div>
  );
}

function PanelTitle({ text, count }: { text: string; count: number }): ReactElement {
  return (
    <h2 style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 17, fontWeight: 800, color: "var(--text)" }}>
      {text}
      <span style={{ minWidth: 26, height: 22, padding: "0 8px", borderRadius: 99, background: "var(--surface-3)", color: "var(--text-muted)", fontSize: 12.5, display: "grid", placeItems: "center" }}>{count}</span>
    </h2>
  );
}
