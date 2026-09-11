"use client";

import type { ChangeEvent, ReactElement } from "react";

import { Field, inputStyle } from "./Field";

/**
 * Los campos de captura del sistema. Todos comparten la misma envoltura
 * (`Field`), así que "obligatorio" y "en rojo" se ven y se anuncian igual en
 * las 23 pantallas.
 *
 * La transformación del texto ocurre en `onChange` y no al pintar: lo que se ve
 * es lo que se guarda. Pintar en mayúsculas y guardar tal cual dejaría "Ana
 * Ruiz" y "ana ruiz" idénticos en pantalla y distintos en la base.
 */
export interface BaseFieldProps {
  label: string;
  value: string;
  onChange: (value: string) => void;
  required?: boolean;
  error?: string;
  /** Se llama al primer tecleo para que el rojo del campo desaparezca. */
  onClearError?: () => void;
  placeholder?: string;
  hint?: string;
  disabled?: boolean;
}

/** Lo que todo campo comparte: id, validez, obligatoriedad y limpiar su rojo. */
function common(
  id: string,
  describedBy: string | undefined,
  p: BaseFieldProps,
): {
  id: string;
  "aria-describedby": string | undefined;
  "aria-invalid": true | undefined;
  "aria-required": true | undefined;
  disabled: boolean | undefined;
  placeholder: string | undefined;
} {
  return {
    id,
    "aria-describedby": describedBy,
    "aria-invalid": p.error !== undefined ? true : undefined,
    "aria-required": p.required === true ? true : undefined,
    disabled: p.disabled,
    placeholder: p.placeholder,
  };
}

export interface TextFieldProps extends BaseFieldProps {
  /**
   * Nombres, conceptos, grados, materias y referencias. NO se usa en el cuerpo
   * de un aviso a los papás: la mayúscula sostenida se lee como gritar.
   */
  uppercase?: boolean;
}

export function TextField(props: TextFieldProps): ReactElement {
  const { label, value, onChange, uppercase, error, onClearError, required, hint } = props;
  return (
    <Field label={label} required={required} error={error} hint={hint}>
      {(id, describedBy) => (
        <input
          {...common(id, describedBy, props)}
          type="text"
          value={value}
          onChange={(e: ChangeEvent<HTMLInputElement>) => {
            // toLocaleUpperCase respeta acentos y la eñe; toUpperCase no siempre.
            onChange(uppercase === true ? e.target.value.toLocaleUpperCase("es-MX") : e.target.value);
            onClearError?.();
          }}
          style={inputStyle(error !== undefined)}
        />
      )}
    </Field>
  );
}

/**
 * El correo, siempre en minúsculas.
 *
 * Se baja al capturar y no al guardar: si sólo se normalizara en el envío,
 * quien escribe "Ana.Ruiz@…" vería una cosa en pantalla y otra en la base, y el
 * día que dos personas lo escriban distinto habría dos cuentas.
 */
export function EmailField(props: BaseFieldProps): ReactElement {
  const { label, value, onChange, error, onClearError, required, hint } = props;
  return (
    <Field label={label} required={required} error={error} hint={hint}>
      {(id, describedBy) => (
        <input
          {...common(id, describedBy, props)}
          type="email"
          inputMode="email"
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          value={value}
          onChange={(e: ChangeEvent<HTMLInputElement>) => {
            onChange(e.target.value.toLowerCase());
            onClearError?.();
          }}
          style={inputStyle(error !== undefined)}
        />
      )}
    </Field>
  );
}

export interface NumberFieldProps extends BaseFieldProps {
  min?: number;
  max?: number;
  step?: number;
}

/**
 * Una cantidad que NO es dinero: una calificación, un cupo, un día del mes.
 * Para dinero está `MoneyField`, que habla en pesos y entrega centavos.
 *
 * El valor viaja como TEXTO. Convertir aquí volvía "" en 0, y un 0 accidental
 * en una calificación es un dato falso que nadie escribió.
 */
export function NumberField(props: NumberFieldProps): ReactElement {
  const { label, value, onChange, error, onClearError, required, hint, min, max, step } = props;
  return (
    <Field label={label} required={required} error={error} hint={hint}>
      {(id, describedBy) => (
        <input
          {...common(id, describedBy, props)}
          type="number"
          inputMode="numeric"
          min={min}
          max={max}
          step={step}
          value={value}
          onChange={(e: ChangeEvent<HTMLInputElement>) => {
            onChange(e.target.value);
            onClearError?.();
          }}
          style={inputStyle(error !== undefined)}
        />
      )}
    </Field>
  );
}

/** El cuerpo de un aviso, una nota, una observación. Nunca en mayúsculas. */
export function TextArea(props: BaseFieldProps & { rows?: number }): ReactElement {
  const { label, value, onChange, error, onClearError, required, hint, rows = 5 } = props;
  return (
    <Field label={label} required={required} error={error} hint={hint}>
      {(id, describedBy) => (
        <textarea
          {...common(id, describedBy, props)}
          rows={rows}
          value={value}
          onChange={(e: ChangeEvent<HTMLTextAreaElement>) => {
            onChange(e.target.value);
            onClearError?.();
          }}
          style={{
            ...inputStyle(error !== undefined),
            height: "auto",
            padding: "12px 14px",
            lineHeight: 1.5,
            resize: "vertical",
          }}
        />
      )}
    </Field>
  );
}

export interface SelectOption {
  value: string;
  label: string;
}

export function SelectField(
  props: BaseFieldProps & { options: readonly SelectOption[] },
): ReactElement {
  const { label, value, onChange, error, onClearError, required, hint, placeholder, options } = props;
  return (
    <Field label={label} required={required} error={error} hint={hint}>
      {(id, describedBy) => {
        // `placeholder` no existe en un <select>: aquí el texto de reposo es una
        // <option> vacía, así que se descarta en vez de reenviarlo al elemento.
        const { placeholder: _unused, ...rest } = common(id, describedBy, props);
        return (
        <select
          {...rest}
          value={value}
          onChange={(e: ChangeEvent<HTMLSelectElement>) => {
            onChange(e.target.value);
            onClearError?.();
          }}
          style={inputStyle(error !== undefined)}
        >
          {placeholder !== undefined && <option value="">{placeholder}</option>}
          {options.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
        );
      }}
    </Field>
  );
}
