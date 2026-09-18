"use client";

import type { ReactElement } from "react";

import { Icon } from "./Icon";

/**
 * Borrar, y dar ocho segundos para arrepentirse.
 *
 * Un diálogo de confirmación en cada borrado entrena a la gente a descartarlo
 * por reflejo — incluidos los que sí importan (NN/g). Y la alternativa que hay
 * hoy es peor: "Quitar" borra en seco, sin red y sin pregunta.
 *
 * Esto no cuesta un clic de más y sí da la red. El diálogo se reserva para lo
 * irreversible; un feriado mal capturado no lo es.
 *
 * El registro NO se borra del servidor hasta que se acaba el plazo: deshacer no
 * es "volverlo a crear", es que nunca llegó a irse.
 *
 * SÓLO ANUNCIA. El plazo lo lleva `useUndoableRemoval`, y esa separación es la
 * corrección de dos defectos que tuvo cuando el reloj vivía aquí: sólo cabía un
 * pendiente —quitar dos cosas seguidas perdía el borrado de la primera— y el
 * efecto colgaba de un `onCommit` escrito en línea, así que cualquier repintado
 * del panel reiniciaba los ocho segundos.
 */
export interface UndoToastProps {
  message: string;
  /** Restituir. Todavía no se había ido a ningún lado. */
  onUndo: () => void;
}

export function UndoToast({ message, onUndo }: UndoToastProps): ReactElement {
  return (
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
    </div>
  );
}
