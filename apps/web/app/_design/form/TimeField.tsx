"use client";

import type { ChangeEvent, ReactElement } from "react";

import { Field, inputStyle } from "./Field";
import type { BaseFieldProps } from "./fields";

const SECONDS_PER_MINUTE = 60;

export interface TimeFieldProps extends BaseFieldProps {
  /** Límites del rango, en HH:MM. */
  min?: string;
  max?: string;
  /** El paso del selector, en minutos. */
  stepMinutes?: number;
}

/**
 * Una hora del día, que se escribe directo (ISSUE-028).
 *
 * Antes era una lista de 288 opciones de 5 en 5 desde las 00:00: para llegar a
 * las 07:00 había que recorrer 84. El navegador pinta `type="time"` con sus
 * casillas de hora y minuto y el valor sigue siendo HH:MM, lo que el API ya
 * recibe. `min` y `max` sólo orientan: un valor guardado fuera de ellos se
 * muestra tal cual, nunca se borra; juzgarlo es de la pantalla.
 */
export function TimeField(props: TimeFieldProps): ReactElement {
  const { label, value, onChange, required, error, onClearError, help, disabled, readOnly, min, max, stepMinutes } = props;
  return (
    <Field label={label} required={required} error={error} help={help}>
      {(id, describedBy) => (
        <input
          id={id}
          type="time"
          lang="es-MX"
          value={value}
          min={min}
          max={max}
          step={stepMinutes !== undefined ? stepMinutes * SECONDS_PER_MINUTE : undefined}
          disabled={disabled}
          readOnly={readOnly}
          aria-describedby={describedBy}
          aria-invalid={error !== undefined ? true : undefined}
          aria-required={required === true ? true : undefined}
          onChange={(e: ChangeEvent<HTMLInputElement>) => {
            onChange(e.target.value);
            onClearError?.();
          }}
          onBlur={props.onBlur}
          style={inputStyle(error !== undefined)}
        />
      )}
    </Field>
  );
}
