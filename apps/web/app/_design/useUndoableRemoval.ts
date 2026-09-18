"use client";

import { useCallback, useEffect, useRef, useState } from "react";

/**
 * Borrar dando ocho segundos para arrepentirse, sin perder ninguno por el camino.
 *
 * El temporizador vive AQUÍ y no dentro del toast, y eso es la corrección: con el
 * plazo adentro del componente que lo anuncia, dos defectos entraban solos.
 *
 *   - Sólo cabía UN pendiente. Quitar un segundo renglón dentro del plazo
 *     sustituía al primero, que desaparecía de la lista y nunca se borraba del
 *     servidor. Para un día cerrado eso deja mal la ventana de pronto pago de
 *     todas las familias, en silencio.
 *   - El efecto dependía de un `onCommit` escrito en línea, así que cualquier
 *     repintado del panel —un mensaje, una recarga— le daba una dependencia nueva
 *     y reiniciaba los ocho segundos. En un panel movido el borrado se podía
 *     posponer para siempre.
 *
 * Y al desmontarse se compromete lo que quede: irse de la pantalla no puede
 * cancelar un borrado que la lista ya mostró como hecho.
 */
const GRACE_MS = 8000;

interface Pending<T> {
  item: T;
  commit: () => void;
  timer: ReturnType<typeof setTimeout>;
}

export interface UndoableRemoval<T> {
  /** Lo último quitado y todavía deshacible, para anunciarlo. */
  readonly latest: T | undefined;
  /** Quita: la lista ya no lo muestra, el servidor todavía no se entera. */
  remove: (item: T, commit: () => void) => void;
  /** Devuelve lo último quitado y cancela su borrado. */
  undo: () => T[] | undefined;
}

export function useUndoableRemoval<T>(): UndoableRemoval<T> {
  const [latest, setLatest] = useState<T | undefined>(undefined);
  const pending = useRef<Pending<T>[]>([]);

  const drop = useCallback((entry: Pending<T>): void => {
    pending.current = pending.current.filter((p) => p !== entry);
    setLatest(pending.current[pending.current.length - 1]?.item);
  }, []);

  const remove = useCallback(
    (item: T, commit: () => void): void => {
      const entry: Pending<T> = {
        item,
        commit,
        timer: setTimeout(() => {
          commit();
          drop(entry);
        }, GRACE_MS),
      };
      pending.current = [...pending.current, entry];
      setLatest(item);
    },
    [drop],
  );

  const undo = useCallback((): T[] | undefined => {
    // Sólo alcanza a lo ÚLTIMO: es lo que el toast está anunciando, y deshacer
    // en montón sorprendería a quien sólo quiso corregir el clic que acaba de dar.
    const entry = pending.current[pending.current.length - 1];
    if (entry === undefined) return undefined;
    clearTimeout(entry.timer);
    drop(entry);
    return [entry.item];
  }, [drop]);

  useEffect(
    () => () => {
      for (const entry of pending.current) {
        clearTimeout(entry.timer);
        entry.commit();
      }
      pending.current = [];
    },
    [],
  );

  return { latest, remove, undo };
}
