"use client";

import { useState, type CSSProperties, type ReactElement } from "react";

import { Icon } from "../../_design/Icon";
import { matchesSearch } from "../../_design/textSearch";
import type { Family } from "../../families/api";
import { accountsLabel } from "../../families/model";
import type { FamilyPupil } from "./familyPupil";

const HEAD: CSSProperties = { fontSize: 12.5, fontWeight: 700, color: "var(--text-faint)", textTransform: "uppercase", letterSpacing: ".04em" };
const OPEN: CSSProperties = { width: 44, height: 44, borderRadius: "50%", display: "grid", placeItems: "center", background: "var(--surface-3)", border: "1px solid var(--border)", color: "var(--text-muted)", cursor: "pointer" };

/** Family, children, app accounts (for guardians:read), open. */
const columns = (accounts: boolean): string => (accounts ? "minmax(0, 1.4fr) minmax(0, 2.4fr) minmax(0, 0.9fr) auto" : "minmax(0, 1.4fr) minmax(0, 2.4fr) auto");

/** The school's families: who is in each, and how many app accounts it holds. */
export function FamiliesTable({ families, pupils, showAccounts, onOpen }: {
  families: readonly Family[];
  pupils: ReadonlyMap<string, FamilyPupil>;
  showAccounts: boolean;
  onOpen: (familyId: string) => void;
}): ReactElement {
  const [query, setQuery] = useState("");
  const shown = families.filter((f) => matchesSearch(query, f.name, ...f.studentIds.map((id) => pupils.get(id)?.name ?? "")));
  const grid: CSSProperties = { display: "grid", gridTemplateColumns: columns(showAccounts), gap: 16, alignItems: "center", padding: "12px 24px" };

  return (
    <section aria-label="Familias" style={{ display: "grid", gap: 14 }}>
      <div style={{ position: "relative", maxWidth: 420 }}>
        <span style={{ position: "absolute", left: 16, top: "50%", transform: "translateY(-50%)", color: "var(--text-faint)" }}><Icon name="search" size={19} /></span>
        <input type="search" value={query} onChange={(e) => setQuery(e.target.value)} aria-label="Buscar familia o hijo" style={{ width: "100%", height: 48, paddingLeft: 46, paddingRight: 16, borderRadius: "var(--r-pill)", border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)", fontSize: 16 }} />
      </div>
      <div role="table" aria-label="Familias de la escuela" style={{ background: "var(--surface)", border: "1px solid var(--border)", borderRadius: "var(--r-xl)", boxShadow: "var(--shadow-card)", overflow: "hidden" }}>
        <div role="row" style={{ ...grid, borderBottom: "1px solid var(--border)" }}>
          <div role="columnheader" style={HEAD}>Familia</div>
          <div role="columnheader" style={HEAD}>Hijos</div>
          {showAccounts && <div role="columnheader" style={HEAD}>Cuentas de app</div>}
          <div role="columnheader" aria-label="Abrir" style={{ width: 44 }} />
        </div>
        {shown.length === 0 && (
          <p style={{ padding: "36px 20px", textAlign: "center", color: "var(--text-muted)", fontSize: 15 }}>
            {families.length === 0 ? "Todavía no hay familias. Empieza por las sugerencias." : "Ninguna familia coincide con la búsqueda."}
          </p>
        )}
        {shown.map((f) => (
          <div role="row" key={f.id} onClick={() => onOpen(f.id)} style={{ ...grid, cursor: "pointer", borderBottom: "1px solid var(--border)" }}>
            <div role="cell" style={{ fontWeight: 700, fontSize: 15.5, color: "var(--text)", minWidth: 0 }}>{f.name}</div>
            <div role="cell" style={{ display: "grid", gap: 2, minWidth: 0 }}>
              {f.studentIds.map((id) => {
                const p = pupils.get(id);
                return (
                  <div key={id} style={{ display: "flex", gap: 8, flexWrap: "wrap", fontSize: 14 }}>
                    <span style={{ color: "var(--text)" }}>{p?.name ?? "Alumno"}</span>
                    {p !== undefined && <span style={{ color: "var(--text-faint)" }}>{p.room}</span>}
                  </div>
                );
              })}
            </div>
            {showAccounts && <div role="cell" style={{ fontSize: 14, color: "var(--text-muted)" }}>{accountsLabel(f.accountCount ?? 0)}</div>}
            <div role="cell">
              <button type="button" aria-label={`Abrir la familia ${f.name}`} onClick={(e) => { e.stopPropagation(); onOpen(f.id); }} style={OPEN}>
                <Icon name="chevron" size={18} />
              </button>
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}
