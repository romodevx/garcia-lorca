"use client";

import type { ChangeEvent, CompositionEvent, ReactElement } from "react";

import { Field, inputStyle } from "./Field";
import { emailShapeError } from "./emailShape";
import { isComposing, lowercaseKeepingCaret } from "./lowercaseKeepingCaret";
import { useFormatError } from "./useFormatError";

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
  /**
   * Qué hay que capturar aquí. Sale en el `(?)` junto a la etiqueta.
   *
   * NO existe `placeholder`, y su ausencia es el contrato: un valor de ejemplo
   * en gris dentro del recuadro ("ADA", "3200.00", "XXXX000000XXXXXX00") es
   * indistinguible de un dato ya capturado. Quien captura duda, y en el peor
   * caso guarda sin escribir. Al no existir la prop, ninguna pantalla puede
   * revivirlo por descuido: el compilador lo impide.
   */
  help?: string;
  disabled?: boolean;
  /**
   * Se lee y se enfoca, pero no se edita: lo que el servidor ya guardó y no se
   * puede corregir desde esta forma. A diferencia de `disabled`, el lector de
   * pantalla lo sigue recorriendo.
   */
  readOnly?: boolean;
  /** Al salir del campo: donde la pantalla acusa un dato mal escrito. */
  onBlur?: () => void;
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
  readOnly: boolean | undefined;
} {
  return {
    id,
    "aria-describedby": describedBy,
    "aria-invalid": p.error !== undefined ? true : undefined,
    "aria-required": p.required === true ? true : undefined,
    disabled: p.disabled,
    readOnly: p.readOnly,
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
  const { label, value, onChange, uppercase, error, onClearError, required, help } = props;
  return (
    <Field label={label} required={required} error={error} help={help}>
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
          onBlur={props.onBlur}
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
  const { label, value, onChange, error, onClearError, required, help } = props;
  // La forma del correo se acusa al salir del campo y se perdona al teclear
  // (useFormatError). El error de quien usa el campo manda sobre éste.
  const shape = useFormatError(EMAIL_SHAPE);
  const shown = error ?? shape.errors.email;
  const change = (next: string): void => {
    onChange(next);
    shape.onChange("email", next, {});
    onClearError?.();
  };
  return (
    <Field label={label} required={required} error={shown} help={help}>
      {(id, describedBy) => (
        <input
          {...common(id, describedBy, { ...props, error: shown })}
          type="text"
          inputMode="email"
          autoComplete="off"
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          value={value}
          onChange={(e: ChangeEvent<HTMLInputElement>) => change(lowercaseKeepingCaret(e.target, isComposing(e)))}
          onCompositionEnd={(e: CompositionEvent<HTMLInputElement>) => change(lowercaseKeepingCaret(e.currentTarget, false))}
          onBlur={() => {
            shape.onBlur("email", value, {});
            props.onBlur?.();
          }}
          style={inputStyle(shown !== undefined)}
        />
      )}
    </Field>
  );
}

/** Fuera del componente: useFormatError lo toma como dependencia. */
const EMAIL_SHAPE = { email: emailShapeError } as const;

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
  const { label, value, onChange, error, onClearError, required, help, min, max, step } = props;
  return (
    <Field label={label} required={required} error={error} help={help}>
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
          onBlur={props.onBlur}
          style={inputStyle(error !== undefined)}
        />
      )}
    </Field>
  );
}

/** El cuerpo de un aviso, una nota, una observación. Nunca en mayúsculas. */
export function TextArea(props: BaseFieldProps & { rows?: number }): ReactElement {
  const { label, value, onChange, error, onClearError, required, help, rows = 5 } = props;
  return (
    <Field label={label} required={required} error={error} help={help}>
      {(id, describedBy) => (
        <textarea
          {...common(id, describedBy, props)}
          rows={rows}
          value={value}
          onChange={(e: ChangeEvent<HTMLTextAreaElement>) => {
            onChange(e.target.value);
            onClearError?.();
          }}
          onBlur={props.onBlur}
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
  const { label, value, onChange, error, onClearError, required, help, options } = props;
  return (
    <Field label={label} required={required} error={error} help={help}>
      {(id, describedBy) => (
        <select
          {...common(id, describedBy, props)}
          value={value}
          onChange={(e: ChangeEvent<HTMLSelectElement>) => {
            onChange(e.target.value);
            onClearError?.();
          }}
          style={inputStyle(error !== undefined)}
        >
          {/*
            La opción vacía va SIEMPRE, y va sin texto.
            Sin ella, el navegador deja preseleccionada la primera opción real y
            el select guarda un nivel, un ciclo o un grupo que nadie eligió. Con
            texto ("Selecciona un nivel…") volvería a parecer un dato capturado:
            lo que hay que capturar lo dice el (?) de la etiqueta.
          */}
          <option value="" />
          {options.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      )}
    </Field>
  );
}
