"use client";

import type { ChangeEvent, ReactElement } from "react";

import { Field, inputStyle } from "./Field";
import type { BaseFieldProps } from "./fields";

export interface DateFieldProps extends BaseFieldProps {
  /** Límites del rango, en aaaa-mm-dd. */
  min?: string;
  max?: string;
}

/**
 * Toda fecha del sistema se elige en un calendario.
 *
 * El navegador la pinta como dd/mm/aaaa en es-MX y abre su propio calendario;
 * el valor sigue siendo ISO aaaa-mm-dd, que es exactamente lo que el API ya
 * recibe. No hay conversión, así que no hay zona horaria de por medio — y este
 * sistema ya tiene un desfase de día conocido en `finance`, que no conviene
 * multiplicar desde la captura.
 *
 * Lo que esto borra: los campos de texto con placeholder "2026-09-20" y el
 * mensaje "la fecha límite se escribe aaaa-mm-dd", un error que con calendario
 * ya no puede ocurrir.
 */
export function DateField(props: DateFieldProps): ReactElement {
  const { label, value, onChange, required, error, onClearError, help, disabled, min, max } = props;
  return (
    <Field label={label} required={required} error={error} help={help}>
      {(id, describedBy) => (
        <input
          id={id}
          type="date"
          lang="es-MX"
          value={value}
          min={min}
          max={max}
          disabled={disabled}
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
