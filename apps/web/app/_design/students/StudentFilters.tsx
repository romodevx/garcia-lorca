"use client";

import type { CSSProperties, ReactElement, ReactNode } from "react";

import { Icon } from "../Icon";
import type { FacetOption, FilterableStudent } from "./studentFilter";
import type { StudentFilter } from "./useStudentFilter";

/** Design system: a target is at least 44 px tall. */
const CHIP_HEIGHT = 44;

const chip = (on: boolean): CSSProperties => ({
  display: "inline-flex", alignItems: "center", gap: 8, height: CHIP_HEIGHT, padding: "0 15px", borderRadius: 99,
  fontSize: 13.5, fontWeight: 600, fontFamily: "var(--font-display)", cursor: "pointer",
  border: `1.5px solid ${on ? "var(--forest)" : "var(--border)"}`,
  background: on ? "color-mix(in oklab, var(--forest) 12%, var(--surface))" : "var(--surface)",
  color: on ? "var(--forest)" : "var(--text-muted)",
});

const badge: CSSProperties = {
  minWidth: 22, height: 20, padding: "0 6px", borderRadius: 99, background: "var(--surface-3)",
  color: "var(--text-muted)", fontSize: 12, display: "grid", placeItems: "center",
};

const rowLabel: CSSProperties = {
  fontSize: 12.5, fontWeight: 700, color: "var(--text-faint)", textTransform: "uppercase", letterSpacing: ".04em", minWidth: 52,
};

/** One row of chips (Nivel, Grado or Grupo); `showCounts` adds each chip's pupil count. */
export function ChipRow({ label, options, chosen, onChoose, trailing, showCounts = true }: {
  label: string;
  options: readonly FacetOption[];
  chosen: string;
  onChoose: (id: string) => void;
  trailing?: ReactNode;
  showCounts?: boolean;
}): ReactElement {
  return (
    <div role="group" aria-label={label} style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
      <span aria-hidden style={rowLabel}>{label}</span>
      {options.map((o) => {
        const on = chosen === o.id;
        return (
          <button key={o.id} type="button" aria-pressed={on} onClick={() => onChoose(o.id)} style={chip(on)}>
            {o.label}
            {showCounts && <span style={badge}>{o.count}</span>}
          </button>
        );
      })}
      {trailing}
    </div>
  );
}

export interface StudentFiltersProps<T extends FilterableStudent> {
  filter: StudentFilter<T>;
  /** Offer "Seleccionar todos (N)" over the filtered pupils. */
  selectable?: boolean;
  /** What the search box says before anything is typed. */
  searchHint?: string;
  /** The screen's own filters (a status tab) are on: "Limpiar" shows for them too. */
  extraActive?: boolean;
  /** Called by "Limpiar" so the screen clears its own filters with these. */
  onClear?: () => void;
  /** Put before the search on the first row (the screen's status tabs). */
  leading?: ReactNode;
  /** Start with the cursor in the search ("Buscar alumno" shortcut). */
  focusSearch?: boolean;
}

/**
 * Level → grade → group chips, chained, and a pill search by surname or first
 * name (T4). Choosing a level shows only its grades; a grade, only its groups.
 * Shared: the students list, the activities picker (T17) and scholarships (T18)
 * render it over their own `useStudentFilter`.
 */
export function StudentFilters<T extends FilterableStudent>({
  filter,
  selectable = false,
  searchHint = "Buscar por apellido o nombre…",
  extraActive = false,
  onClear,
  leading,
  focusSearch = false,
}: StudentFiltersProps<T>): ReactElement {
  const { facets, state } = filter;
  const clearable = filter.active || extraActive;
  const clear = (): void => {
    filter.clear();
    onClear?.();
  };

  return (
    // minmax(0, 1fr): a row wider than a phone scrolls inside itself, never the page.
    <div style={{ display: "grid", gridTemplateColumns: "minmax(0, 1fr)", gap: 12 }}>
      <div style={{ display: "flex", alignItems: "center", gap: 14, flexWrap: "wrap" }}>
        {leading}
        <div style={{ position: "relative", flex: "1 1 230px", minWidth: 0, maxWidth: 420 }}>
          <span style={{ position: "absolute", left: 16, top: "50%", transform: "translateY(-50%)", color: "var(--text-faint)" }}><Icon name="search" size={19} /></span>
          <input
            type="search"
            autoFocus={focusSearch}
            value={state.q}
            onChange={(e) => filter.setQuery(e.target.value)}
            aria-label="Buscar"
            placeholder={searchHint}
            style={{ width: "100%", height: 48, paddingLeft: 46, paddingRight: 16, borderRadius: "var(--r-pill)", border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)", fontSize: 16, outline: "none" }}
          />
        </div>
      </div>

      <ChipRow
        label="Nivel"
        options={facets.levels}
        chosen={state.levelId}
        onChoose={filter.chooseLevel}
        trailing={clearable && (
          <button type="button" onClick={clear} style={{ height: CHIP_HEIGHT, padding: "0 14px", borderRadius: 99, display: "inline-flex", alignItems: "center", gap: 6, fontSize: 13, fontWeight: 600, color: "var(--text-muted)", background: "var(--surface-3)", border: "1px solid var(--border)", cursor: "pointer" }}>
            <Icon name="close" size={15} />Limpiar
          </button>
        )}
      />
      {facets.grades.length > 0 && <ChipRow label="Grado" options={facets.grades} chosen={state.gradeId} onChoose={filter.chooseGrade} />}
      {facets.groups.length > 0 && <ChipRow label="Grupo" options={facets.groups} chosen={state.groupId} onChoose={filter.chooseGroup} />}

      {selectable && filter.filtered.length > 0 && (
        <div>
          <button
            type="button"
            aria-pressed={filter.allSelected}
            onClick={filter.selectAll}
            style={{ ...chip(filter.allSelected), borderColor: filter.allSelected ? "var(--lime)" : "var(--border)", color: filter.allSelected ? "var(--text)" : "var(--text-muted)", background: filter.allSelected ? "color-mix(in oklab, var(--lime) 18%, var(--surface))" : "var(--surface)" }}
          >
            <Checkmark on={filter.allSelected} />
            Seleccionar todos ({filter.filtered.length})
          </button>
        </div>
      )}
    </div>
  );
}

/** The square a row or "select all" shows: empty, or lime with a check. */
export function Checkmark({ on }: { on: boolean }): ReactElement {
  return (
    <span aria-hidden style={{ width: 20, height: 20, borderRadius: 6, flexShrink: 0, display: "grid", placeItems: "center", border: `1.5px solid ${on ? "var(--lime)" : "var(--border-strong)"}`, background: on ? "var(--lime)" : "var(--surface)", color: "var(--primary-ink)" }}>
      {on && <Icon name="check" size={14} stroke={3} />}
    </span>
  );
}

/**
 * The lime band that appears while pupils are picked (design system §6): how
 * many, what can be done with them, and a way to let go.
 */
export function StudentSelectionBand<T extends FilterableStudent>({ filter, children }: { filter: StudentFilter<T>; children?: ReactNode }): ReactElement | null {
  const n = filter.selected.size;
  if (n === 0) return null;
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap", padding: "10px 16px", borderRadius: "var(--r-lg)", background: "color-mix(in oklab, var(--lime) 22%, var(--surface))", border: "1.5px solid var(--lime)" }}>
      <span role="status" style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 15, color: "var(--text)" }}>
        {n === 1 ? "1 seleccionado" : `${n} seleccionados`}
      </span>
      <div style={{ flex: 1 }} />
      {children}
      <button type="button" onClick={filter.clearSelection} style={{ height: CHIP_HEIGHT, padding: "0 14px", borderRadius: 99, display: "inline-flex", alignItems: "center", gap: 6, fontSize: 13.5, fontWeight: 600, color: "var(--text)", background: "transparent", border: "1px solid color-mix(in oklab, var(--text) 20%, transparent)", cursor: "pointer" }}>
        <Icon name="close" size={15} />Quitar selección
      </button>
    </div>
  );
}
