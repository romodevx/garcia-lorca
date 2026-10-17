"use client";

import { useEffect, useRef, useState, type ReactElement } from "react";

import { InlineCreateRow } from "../../_design/InlineCreateRow";
import { API_URL } from "../../api-url";
import { createCategory, updateCategory, type ExpenseCategory } from "../../expenses/api";

/**
 * The school's own categories: one field, entered repeatedly — so an inline
 * row, not a drawer (forms rule). Deactivated, never deleted: recorded
 * expenses keep pointing at them.
 */
export function CategoriesPanel({
  categories,
  canWrite,
  onChanged,
  focusNew = false,
}: {
  categories: readonly ExpenseCategory[];
  canWrite: boolean;
  onChanged: () => void;
  /** Arrived from "Crear la primera categoría": the cursor waits on "Nueva categoría". */
  focusNew?: boolean;
}): ReactElement {
  const [error, setError] = useState<string | undefined>(undefined);
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!focusNew) return;
    const add = [...(root.current?.querySelectorAll("button") ?? [])].find((b) => b.textContent?.includes("Nueva categoría"));
    add?.focus();
  }, [focusNew]);

  const add = async (values: Record<string, string>): Promise<void> => {
    setError(undefined);
    try {
      await createCategory(API_URL, (values.name ?? "").trim());
      onChanged();
    } catch (err) {
      setError(err instanceof Error ? err.message : "No se pudo crear la categoría.");
      throw err;
    }
  };

  const toggle = async (c: ExpenseCategory): Promise<void> => {
    setError(undefined);
    try {
      await updateCategory(API_URL, c.id, { name: c.name, active: !c.active });
      onChanged();
    } catch (err) {
      setError(err instanceof Error ? err.message : "No se pudo cambiar la categoría.");
    }
  };

  return (
    <div ref={root} style={{ display: "flex", flexDirection: "column", gap: 12, padding: 20, borderRadius: "var(--r-xl)", background: "var(--surface)", border: "1px solid var(--border)", boxShadow: "var(--shadow-card)" }}>
      <p style={{ margin: 0, fontSize: 14, color: "var(--text-muted)" }}>
        Las categorías ordenan los egresos en el balance y los reportes. Una categoría desactivada ya no se ofrece al registrar, pero sus egresos se conservan.
      </p>
      {categories.length === 0 && <p style={{ margin: 0, fontSize: 15, color: "var(--text-muted)" }}>Todavía no hay categorías. Crea la primera para registrar egresos.</p>}
      <div style={{ borderRadius: "var(--r-md)", border: "1px solid var(--border)", overflow: "hidden" }}>
        {categories.map((c) => (
          <div key={c.id} data-testid="category-row" style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12, padding: "10px 16px", borderTop: "1px solid var(--border)" }}>
            <span style={{ fontSize: 15, fontWeight: 600, color: c.active ? "var(--text)" : "var(--text-faint)" }}>
              {c.name}
              {!c.active && <span style={{ marginLeft: 8, fontSize: 12.5, fontWeight: 600 }}>desactivada</span>}
            </span>
            {canWrite && (
              <button type="button" aria-label={`${c.active ? "Desactivar" : "Activar"} ${c.name}`} onClick={() => void toggle(c)}
                style={{ minHeight: 44, padding: "0 14px", borderRadius: 99, border: "1px solid var(--border)", background: "var(--surface-3)", color: "var(--text)", fontWeight: 600 }}>
                {c.active ? "Desactivar" : "Activar"}
              </button>
            )}
          </div>
        ))}
      </div>
      {canWrite && (
        <InlineCreateRow
          addLabel="Nueva categoría"
          columns={[{ key: "name", label: "Nombre", kind: "text", uppercase: true, required: true, help: "Cómo se llama el tipo de gasto: PAPELERÍA, MANTENIMIENTO, NÓMINA." }]}
          onCreate={add}
        />
      )}
      {error !== undefined && <p role="alert" style={{ margin: 0, color: "var(--bad-fg)", fontSize: 14 }}>{error}</p>}
    </div>
  );
}
