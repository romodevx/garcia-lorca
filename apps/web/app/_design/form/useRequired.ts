"use client";

import { useCallback, useState } from "react";

import { useFocusFirstInvalid } from "./useFocusFirstInvalid";

/** El mensaje que se pinta bajo un campo obligatorio vacío. */
export const FALTA = "Falta este dato";

export interface RequiredState {
  /** Los campos marcados en rojo ahora mismo, por clave. */
  readonly errors: Readonly<Record<string, string>>;
  /** Revisa y marca. Devuelve true si se puede guardar. */
  check: (values: Readonly<Record<string, string | undefined>>) => boolean;
  /** Limpia el rojo de un campo — se llama desde su onChange. */
  clear: (key: string) => void;
}

/**
 * Qué falta para poder guardar.
 *
 * El momento importa: NO reclama mientras se captura, sólo al guardar, y
 * entonces marca todos los faltantes a la vez. Reclamar campo por campo al
 * salir de cada uno persigue con rojos a quien apenas va llegando al siguiente.
 *
 * Un `check` que falla manda además el foco al primer campo en rojo y lo despeja
 * del pie del cajón (ver `useFocusFirstInvalid`). Ese salto lo resuelve el DOM y
 * no este hook, a propósito: en el sistema conviven dos validadores y los dos
 * acaban marcando `aria-invalid`, así que el DOM es donde se encuentran — y su
 * orden es el que el ojo sigue bajando por la forma.
 *
 * Aquí vivieron `firstMissing` y `missingCount`: calculados, probados y jamás
 * consumidos. El primero decía en su comentario "a donde salta la vista" y la
 * vista no saltaba. Se fueron con el arreglo; lo vigila `contratos.test.ts`.
 */
export function useRequired(required: readonly string[]): RequiredState {
  const [errors, setErrors] = useState<Record<string, string>>({});
  const señalarPrimerError = useFocusFirstInvalid();

  const check = useCallback(
    (values: Readonly<Record<string, string | undefined>>): boolean => {
      const found: Record<string, string> = {};
      for (const key of required) {
        if ((values[key] ?? "").trim() === "") {
          found[key] = FALTA;
        }
      }
      setErrors(found);
      const ok = Object.keys(found).length === 0;
      // Marcar en rojo no basta: lo que falta puede estar debajo del pie del
      // cajón, donde no se ve y donde el teclado no llega solo.
      if (!ok) señalarPrimerError();
      return ok;
    },
    [required, señalarPrimerError],
  );

  const clear = useCallback((key: string): void => {
    setErrors((prev) => {
      // La misma referencia cuando no hay nada que limpiar: esto se llama en
      // cada tecla, y devolver un objeto nuevo repintaría la forma entera.
      if (prev[key] === undefined) return prev;
      const rest = { ...prev };
      delete rest[key];
      return rest;
    });
  }, []);

  return { errors, check, clear };
}
