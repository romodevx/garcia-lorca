"use client";

import type { ChangeEvent, ReactElement } from "react";

import { centsToPesosInput, pesosToCents } from "../../_lib/money";
import { Field, inputStyle } from "./Field";
import type { BaseFieldProps } from "./fields";

/** Ancho del prefijo "$" y su separación del texto. */
const PREFIX_WIDTH = 30;

/** Agrupa de tres en tres la parte entera: 1850.00 → 1,850.00 */
function withThousands(plain: string): string {
  const [whole = "0", decimals = "00"] = plain.split(".");
  return `${whole.replace(/\B(?=(\d{3})+(?!\d))/g, ",")}.${decimals}`;
}

/**
 * Un monto: se ve en pesos, viaja en centavos.
 *
 * El formateo ocurre al SALIR del campo, no mientras se teclea: reacomodar las
 * comas bajo los dedos mueve el cursor y hace escribir el número dos veces.
 *
 * Cuando lo escrito no es un monto reconocible, el campo NO se toca. Corregirlo
 * a algo plausible es peor que dejarlo: quien escribió "1850,50" con coma
 * decimal debe VER su texto y arreglarlo, no descubrir después que el sistema
 * entendió otra cifra. `pesosToCents` es estricto exactamente por eso — esa
 * coma, leída a la ligera, cobraba cien veces de más.
 *
 * Lo que cruza al API siguen siendo centavos enteros: el consumidor llama
 * `pesosToCents` al guardar. Ningún flotante regresa.
 */
export function MoneyField(props: BaseFieldProps): ReactElement {
  const { label, value, onChange, required, error, onClearError, help, disabled } = props;

  const format = (): void => {
    if (value.trim() === "") return;
    const cents = pesosToCents(value);
    if (Number.isNaN(cents)) return;
    const formatted = withThousands(centsToPesosInput(cents));
    if (formatted !== value) onChange(formatted);
  };

  return (
    <Field label={label} required={required} error={error} help={help}>
      {(id, describedBy) => (
        <div style={{ position: "relative", minWidth: 0 }}>
          <span
            aria-hidden="true"
            style={{
              position: "absolute",
              left: 14,
              top: "50%",
              transform: "translateY(-50%)",
              color: "var(--text-faint)",
              fontSize: 15,
              pointerEvents: "none",
            }}
          >
            $
          </span>
          <input
            id={id}
            type="text"
            inputMode="decimal"
            value={value}
            disabled={disabled}
            aria-describedby={describedBy}
            aria-invalid={error !== undefined ? true : undefined}
            aria-required={required === true ? true : undefined}
            onChange={(e: ChangeEvent<HTMLInputElement>) => {
              onChange(e.target.value);
              onClearError?.();
            }}
            onBlur={format}
            style={{ ...inputStyle(error !== undefined), paddingLeft: PREFIX_WIDTH }}
          />
        </div>
      )}
    </Field>
  );
}
