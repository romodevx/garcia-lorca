"use client";

import { useCallback, useMemo, useState } from "react";

import { useFocusFirstInvalid } from "./useFocusFirstInvalid";

/** El mensaje que se pinta bajo un campo obligatorio vacío. */
export const FALTA = "Falta este dato";

export interface RequiredState {
  /** Los campos marcados en rojo ahora mismo, por clave. */
  readonly errors: Readonly<Record<string, string>>;
  /** El primero que falta EN EL ORDEN DECLARADO — a donde salta la vista. */
  readonly firstMissing: string | undefined;
  /** Cuántos faltan, para el resumen de arriba. */
  readonly missingCount: number;
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
 * `firstMissing` respeta el orden en que se DECLARARON los obligatorios, no el
 * orden de las llaves del objeto: la vista debe saltar al primero que el ojo
 * encuentra bajando por la forma, y el orden de un objeto no es el de la
 * pantalla.
 *
 * Y saltar es literal: un `check` que falla manda el foco al primer campo en
 * rojo y lo despeja del pie del cajón (ver `useFocusFirstInvalid`). Durante un
 * tiempo esto sólo se calculó y nadie lo usó, y por eso el error de "Tutor" en
 * el alta de alumno era invisible.
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

  const firstMissing = useMemo(
    () => required.find((key) => errors[key] !== undefined),
    [required, errors],
  );

  return {
    errors,
    firstMissing,
    missingCount: Object.keys(errors).length,
    check,
    clear,
  };
}
