"use client";

import { useCallback, useState } from "react";

/**
 * Lo que está MAL ESCRITO, revisado al salir del campo.
 *
 * Deliberadamente NO revisa lo que FALTA: de eso se encarga `useRequired`, al
 * guardar, y esa decisión está bien tomada — reclamar un campo vacío al salir de
 * él persigue con rojos a quien apenas va llegando al siguiente.
 *
 * Lo que sí gana con revisarse en `blur` es el dato imposible: una fecha de fin
 * anterior a la de inicio, un monto negativo. Ahí el dato ya está escrito y la
 * persona todavía está parada en el campo, que es el momento en que corregirlo
 * es barato. Esa es la distinción que el hallazgo de Baymard hace y que es fácil
 * leer al revés: valida al salir, nunca mientras se teclea.
 *
 * Y la simetría importa: se ACUSA al salir, pero se PERDONA por tecla. En cuanto
 * el valor vuelve a ser válido el rojo se va solo, sin obligar a salir otra vez
 * para que alguien se digne a quitarlo.
 */
export type FormatRule = (
  value: string,
  all: Readonly<Record<string, string>>,
) => string | undefined;

export interface FormatErrorState {
  readonly errors: Readonly<Record<string, string>>;
  /** Al salir del campo: revisa y, si está mal, marca. */
  onBlur: (key: string, value: string, all: Readonly<Record<string, string>>) => void;
  /** Al teclear: sólo perdona, nunca acusa. */
  onChange: (key: string, value: string, all: Readonly<Record<string, string>>) => void;
  /**
   * Al GUARDAR: revisa todas las reglas de una vez y dice si se puede seguir.
   *
   * Hace falta porque `blur` no cubre el envío: Enter guarda desde dentro de la
   * celda que se está escribiendo, y esa celda nunca pierde el foco. Sin esto, la
   * regla existía y el dato imposible salía igual — el caso exacto que el renglón
   * de días cerrados pretendía impedir.
   */
  checkAll: (all: Readonly<Record<string, string>>) => boolean;
  /** Borrón y cuenta nueva: lo pide quien cancela la captura. */
  reset: () => void;
}

export function useFormatError(
  rules: Readonly<Record<string, FormatRule>>,
): FormatErrorState {
  const [errors, setErrors] = useState<Record<string, string>>({});

  const evaluate = useCallback(
    (
      key: string,
      value: string,
      all: Readonly<Record<string, string>>,
      accusing: boolean,
    ): void => {
      const rule = rules[key];
      if (rule === undefined) return;
      // Un campo vacío nunca es "mal escrito". Eso lo dice useRequired, al guardar.
      const found = value.trim() === "" ? undefined : rule(value, all);

      setErrors((prev) => {
        if (found === undefined) {
          // La misma referencia cuando no hay nada que limpiar: esto corre en
          // cada tecla y un objeto nuevo repintaría la forma entera.
          if (prev[key] === undefined) return prev;
          const rest = { ...prev };
          delete rest[key];
          return rest;
        }
        if (!accusing || prev[key] === found) return prev;
        return { ...prev, [key]: found };
      });
    },
    [rules],
  );

  const checkAll = useCallback(
    (all: Readonly<Record<string, string>>): boolean => {
      const found: Record<string, string> = {};
      for (const [key, rule] of Object.entries(rules)) {
        const value = all[key] ?? "";
        // Un campo vacío nunca es "mal escrito": eso lo dice useRequired.
        if (value.trim() === "") continue;
        const problem = rule(value, all);
        if (problem !== undefined) found[key] = problem;
      }
      setErrors(found);
      return Object.keys(found).length === 0;
    },
    [rules],
  );

  const reset = useCallback((): void => setErrors({}), []);

  return {
    errors,
    checkAll,
    reset,
    onBlur: useCallback(
      (key, value, all) => evaluate(key, value, all, true),
      [evaluate],
    ),
    onChange: useCallback(
      (key, value, all) => evaluate(key, value, all, false),
      [evaluate],
    ),
  };
}
