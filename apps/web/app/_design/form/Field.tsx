"use client";

import { useId, type CSSProperties, type ReactElement } from "react";

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
  children: (id: string, describedBy: string | undefined) => ReactElement;
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

export function Field({ label, required, error, help, children }: FieldProps): ReactElement {
  const id = useId();
  const errorId = `${id}-error`;
  const helpId = `${id}-help`;
  // El error gana sobre la ayuda: cuando algo está mal, lo que hay que leer es
  // qué está mal, no cómo se llenaba el campo. El (?) sigue ahí para consultarlo.
  const describedBy = error !== undefined ? errorId : help !== undefined ? helpId : undefined;

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
    outline: "none",
    width: "100%",
    minWidth: 0,
  };
}
