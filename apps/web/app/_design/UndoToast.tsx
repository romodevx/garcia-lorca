"use client";

import type { ReactElement } from "react";

import { BodyPortal } from "./BodyPortal";
import { Icon } from "./Icon";

/**
 * Borrar, y dejar el Deshacer a mano hasta que la persona siga con otra cosa.
 *
 * Un diálogo de confirmación en cada borrado entrena a la gente a descartarlo
 * por reflejo — incluidos los que sí importan (NN/g). Y la alternativa que hay
 * hoy es peor: "Quitar" borra en seco, sin red y sin pregunta.
 *
 * Esto no cuesta un clic de más y sí da la red. El diálogo se reserva para lo
 * irreversible; un feriado mal capturado no lo es.
 *
 * NO se cierra solo (ISSUE-CC-105, regla de la casa): un aviso que se va a los
 * ocho segundos deja sin Deshacer a quien leía despacio. Se va cuando la persona
 * lo cierra (`onDismiss`), quita otra cosa o deja la pantalla. Lo diferido lo
 * compromete `useUndoableRemoval`; este componente SÓLO anuncia.
 *
 * Se pinta en `document.body` (portal): dentro de un panel escondido el
 * Deshacer desaparecería mientras el plazo sigue corriendo y confirma.
 */
export interface UndoToastProps {
  message: string;
  /** Restituir. Todavía no se había ido a ningún lado. */
  onUndo: () => void;
  /** La persona cerró el aviso: lo quitado se queda quitado. */
  onDismiss: () => void;
}

export function UndoToast({ message, onUndo, onDismiss }: UndoToastProps): ReactElement {
  return (
    <BodyPortal>
      <div
        // `status` y no `alert`: se anuncia, pero no interrumpe ni roba el foco de
        // donde la persona está trabajando.
        role="status"
        style={{
          position: "fixed",
          bottom: 28,
          left: "50%",
          transform: "translateX(-50%)",
          zIndex: 80,
          display: "flex",
          alignItems: "center",
          gap: 14,
          padding: "12px 14px 12px 22px",
          borderRadius: "var(--r-pill)",
          background: "var(--text)",
          color: "var(--surface)",
          fontWeight: 600,
          fontSize: 15,
          boxShadow: "var(--shadow-pop)",
          animation: "pop .25s var(--ease) both",
        }}
      >
        <Icon name="check" size={20} stroke={2.6} />
        {message}
        <button
          type="button"
          onClick={onUndo}
          style={{
            display: "inline-flex",
            alignItems: "center",
            height: 38,
            padding: "0 18px",
            borderRadius: "var(--r-pill)",
            background: "var(--lime)",
            color: "#16311A",
            fontFamily: "var(--font-display)",
            fontWeight: 700,
            fontSize: 14.5,
            cursor: "pointer",
            border: "none",
          }}
        >
          Deshacer
        </button>
        <button
          type="button"
          aria-label="Cerrar aviso"
          onClick={onDismiss}
          style={{
            display: "inline-grid",
            placeItems: "center",
            width: 44,
            height: 44,
            borderRadius: "var(--r-pill)",
            background: "transparent",
            color: "inherit",
            cursor: "pointer",
            border: "none",
          }}
        >
          <Icon name="close" size={18} stroke={2.4} />
        </button>
      </div>
    </BodyPortal>
  );
}
