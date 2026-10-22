"use client";

import { useId, type CSSProperties, type ReactElement, type ReactNode } from "react";

import { Icon } from "../Icon";
import { HelpTip } from "./HelpTip";

/**
 * La envoltura de todo campo del sistema: etiqueta, el asterisco de
 * obligatorio, el borde rojo y el mensaje ligado por ARIA.
 *
 * Existe para que "requerido" y "en rojo" signifiquen lo mismo en las 23
 * pantallas. Antes cada una dibujaba su propio input en línea y sólo dos
 * lugares en toda la web decían `required`.
 *
 * Los hijos llegan como FUNCIÓN `(id, describedBy)` y no como elemento: así el
 * campo recibe su `id` y su `aria-describedby` sin que Field tenga que clonar
 * un elemento ajeno y adivinar qué props ya traía.
 */
export interface FieldProps {
  label: string;
  required?: boolean;
  error?: string;
  /**
   * Qué hay que capturar aquí. Sale en el `(?)` de junto a la etiqueta.
   *
   * Sustituye a los VALORES DE EJEMPLO que vivían dentro del campo ("ADA",
   * "3200.00"): en gris claro, dentro del recuadro, eran indistinguibles de un
   * dato ya capturado.
   */
  help?: string;
  /**
   * A non-blocking warning (PLAN-import-avisos): amber, announced politely,
   * tied to the field by aria-describedby and never `aria-invalid`. The error
   * wins: while there is one, the warning is not shown.
   */
  warning?: string;
  /** What can be done about the warning ("Está bien así"), after its text. */
  warningAction?: ReactNode;
  children: (id: string, describedBy: string | undefined) => ReactElement;
}

const warningStyle: CSSProperties = {
  display: "flex",
  alignItems: "flex-start",
  gap: 6,
  fontSize: 13,
  fontWeight: 600,
  lineHeight: 1.45,
  color: "var(--warn-fg)",
  background: "var(--warn-bg)",
  borderRadius: "var(--r-md)",
  padding: "8px 10px",
  margin: 0,
};

/** The warning on its own, for a value that has no single field (a dropped phone). */
export function FieldWarning({ id, text, action }: { id?: string; text: string; action?: ReactNode }): ReactElement {
  return (
    <div style={{ display: "grid", gap: 6, marginTop: 6 }}>
      <p id={id} role="status" aria-live="polite" style={warningStyle}>
        <span aria-hidden="true" style={{ flexShrink: 0, display: "inline-flex", paddingTop: 1 }}>
          <Icon name="warning" size={14} stroke={2.4} />
        </span>
        {text}
      </p>
      {action}
    </div>
  );
}

const labelRowStyle: CSSProperties = {
  display: "flex",
  alignItems: "baseline",
  gap: 6,
  marginBottom: 6,
};

const labelStyle: CSSProperties = {
  fontSize: 13,
  fontWeight: 600,
  color: "var(--text-muted)",
};

export function Field({ label, required, error, help, warning, warningAction, children }: FieldProps): ReactElement {
  const id = useId();
  const errorId = `${id}-error`;
  const helpId = `${id}-help`;
  const warningId = `${id}-warning`;
  // El error gana sobre la ayuda: cuando algo está mal, lo que hay que leer es
  // qué está mal, no cómo se llenaba el campo. El (?) sigue ahí para consultarlo.
  // Y gana sobre el aviso: un dato inválido se corrige antes de discutirlo.
  const warned = error === undefined && warning !== undefined;
  const describedBy =
    error !== undefined
      ? errorId
      : [warned ? warningId : undefined, help !== undefined ? helpId : undefined].filter((v) => v !== undefined).join(" ") || undefined;

  return (
    <div style={{ display: "block", minWidth: 0 }}>
      {/*
        El asterisco es HERMANO del <label>, no su hijo, y eso es deliberado:
        dentro, el nombre del campo pasaría a ser "Nombre *" para cualquiera que
        lo busque por su etiqueta — incluidas las pruebas de las 23 pantallas.
        Fuera, el nombre sigue siendo "Nombre" y el asterisco sólo se ve.
      */}
      <div style={labelRowStyle}>
        <label htmlFor={id} style={labelStyle}>
          {label}
        </label>
        {required === true && (
          <span aria-hidden="true" style={{ color: "var(--red)", fontWeight: 700 }}>
            *
          </span>
        )}
        {help !== undefined && <HelpTip id={helpId} label={label} text={help} />}
      </div>
      {children(id, describedBy)}
      {error !== undefined && (
        <p
          id={errorId}
          role="alert"
          style={{
            display: "flex",
            alignItems: "center",
            gap: 5,
            fontSize: 12.5,
            fontWeight: 600,
            color: "var(--bad-fg)",
            marginTop: 5,
          }}
        >
          <Icon name="warning" size={14} stroke={2.4} />
          {error}
        </p>
      )}
      {warned && <FieldWarning id={warningId} text={warning} action={warningAction} />}
    </div>
  );
}

/**
 * El aspecto de un campo.
 *
 * El rojo NO es sólo el borde: lleva halo. Un borde de 1px en una forma de
 * veinte campos no se encuentra de un vistazo, y encontrarlo rápido es
 * justamente lo que se pidió.
 */
export function inputStyle(invalid: boolean): CSSProperties {
  return {
    height: 44,
    padding: "0 14px",
    borderRadius: "var(--r-md)",
    border: `1px solid ${invalid ? "var(--red)" : "var(--border)"}`,
    boxShadow: invalid ? "0 0 0 3px color-mix(in oklab, var(--red) 22%, transparent)" : "none",
    background: "var(--surface)",
    color: "var(--text)",
    fontFamily: "var(--font-ui)",
    fontSize: 15,
    // No `outline` here: an inline value beats globals.css's :focus-visible
    // ring and leaves a keyboard user with no idea where focus is.
    width: "100%",
    minWidth: 0,
  };
}
