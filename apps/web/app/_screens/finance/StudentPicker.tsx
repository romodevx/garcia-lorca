"use client";

import { useMemo, useState, type CSSProperties, type ReactElement } from "react";

import { DrawerShell } from "../../_design/DrawerShell";
import { Icon } from "../../_design/Icon";
import type { CollectStudent } from "./CollectDrawer";

const search: CSSProperties = {
  width: "100%",
  height: 60,
  paddingLeft: 52,
  paddingRight: 18,
  border: "none",
  background: "transparent",
  color: "var(--text)",
  fontFamily: "var(--font-ui)",
  fontSize: 18,
  outline: "none",
};

export interface StudentPickerProps {
  students: CollectStudent[];
  onPick: (student: CollectStudent) => void;
  onClose: () => void;
}

/**
 * Find the pupil, then take the money.
 *
 * The counter's entry point is a person, not a charge. A cashier is handed cash
 * by a parent who says a name; making them find the right row of the right month
 * first is what sent the office back to the notebook.
 *
 * At most ten names are shown. A list longer than that is not read, it is
 * scrolled — and the answer is to type another letter.
 *
 * MONTA <DrawerShell>, y no su propio diálogo, aunque no se parezca a un alta.
 *
 * Tuvo el suyo hasta sep-2026, y se le notaba en lo que le faltaba: sin trampa
 * de foco —tabular se salía a la página de atrás con el buscador abierto— y sin
 * devolver el foco al botón que lo abrió al cerrar. Nada de eso es propio de un
 * buscador: es el contrato de diálogo de APG, y vive una sola vez en la cáscara.
 *
 * Lo que sí es propio suyo entra por `header`: aquí no hay ícono ni título, sino
 * el campo de búsqueda, con la lista creciendo debajo. Y `padded={false}` porque
 * esa lista trae su propio desplazamiento; anidar dos zonas de desplazamiento
 * deja la de afuera rodando sobre la de adentro.
 */
export function StudentPicker({ students, onPick, onClose }: StudentPickerProps): ReactElement | null {
  const [q, setQ] = useState("");

  const matches = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return students
      .filter((s) => needle === "" || `${s.firstName} ${s.lastName}`.toLowerCase().includes(needle))
      .slice(0, 10);
  }, [students, q]);

  return (
    <DrawerShell
      // Sin encabezado propio no se dibuja, pero nombra al diálogo: es lo que
      // anuncia un lector de pantalla y lo que buscan las pruebas.
      title="Buscar alumno para cobrar"
      icon="search"
      padded={false}
      onClose={onClose}
      header={
        <div style={{ position: "relative", borderBottom: "1px solid var(--border)" }}>
          <span
            style={{
              position: "absolute",
              left: 20,
              top: "50%",
              transform: "translateY(-50%)",
              color: "var(--text-faint)",
            }}
          >
            <Icon name="search" size={21} />
          </span>
          {/*
            Sin `autoFocus`: la cáscara pone el foco en el primer campo al abrir,
            y ese campo es éste. Dos manos sobre el mismo foco es como se termina
            con un cursor que parpadea donde nadie lo pidió.
          */}
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            aria-label="Buscar alumno"
            placeholder="Nombre del alumno…"
            style={search}
          />
        </div>
      }
    >
      <div style={{ maxHeight: "48vh", overflowY: "auto" }}>
        {matches.length === 0 && (
          <p style={{ padding: "22px 24px", margin: 0, color: "var(--text-muted)", fontSize: 15 }}>
            Ningún alumno con ese nombre.
          </p>
        )}
        {matches.map((s) => (
          <button
            key={s.id}
            onClick={() => onPick(s)}
            style={{
              display: "flex",
              width: "100%",
              alignItems: "center",
              gap: 12,
              padding: "14px 24px",
              background: "transparent",
              border: "none",
              borderTop: "1px solid var(--border)",
              cursor: "pointer",
              textAlign: "left",
            }}
          >
            <span style={{ fontSize: 15.5, fontWeight: 600, color: "var(--text)" }}>
              {s.firstName} {s.lastName}
            </span>
            <span style={{ fontSize: 13.5, color: "var(--text-muted)" }}>{s.gradeGroup ?? ""}</span>
          </button>
        ))}
      </div>
    </DrawerShell>
  );
}
