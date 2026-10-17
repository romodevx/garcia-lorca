"use client";

import { useCallback, useEffect, useRef, useState } from "react";

/**
 * Borrar dejando el Deshacer a mano hasta que la persona siga con otra cosa.
 *
 * NO hay temporizador (ISSUE-CC-105). Antes el aviso se iba solo a los ocho
 * segundos y en ese instante se ejecutaba el borrado, aunque la persona siguiera
 * leyendo. La regla de la casa (ui-forms-and-modals) pide que el aviso de
 * Deshacer no se cierre solo, así que lo quitado se compromete cuando:
 *
 *   - la persona cierra el aviso (`dismiss`),
 *   - quita otra cosa (`remove` compromete lo anterior: ya no se anuncia),
 *   - la pantalla lanza otra acción (`flush`),
 *   - se va de la pantalla (desmontaje) o cierra la pestaña (`pagehide`).
 *
 * Nada se pierde ni se borra dos veces: cada pendiente sale de la cola antes de
 * comprometerse.
 */
interface Pending<T> {
  item: T;
  commit: () => void;
}

export interface UndoableRemoval<T> {
  /** Lo último quitado y todavía deshacible, para anunciarlo. */
  readonly latest: T | undefined;
  /**
   * TODO lo quitado y todavía no borrado del servidor.
   *
   * Quien lo use tiene que filtrarlo de lo que vuelva a pedirle al servidor: la
   * lista de allá todavía lo trae, así que una recarga con el aviso abierto lo
   * resucitaría en pantalla.
   */
  readonly pending: readonly T[];
  /** Quita: la lista ya no lo muestra, el servidor todavía no se entera. */
  remove: (item: T, commit: () => void) => void;
  /** Devuelve lo último quitado y cancela su borrado. */
  undo: () => T[] | undefined;
  /** La persona cerró el aviso: se borra de verdad. */
  dismiss: () => void;
  /** Otra acción empezó: lo pendiente deja de ser deshacible y se borra. */
  flush: () => void;
}

export function useUndoableRemoval<T>(): UndoableRemoval<T> {
  const [items, setItems] = useState<readonly T[]>([]);
  const pending = useRef<Pending<T>[]>([]);

  const publish = useCallback((): void => {
    setItems(pending.current.map((p) => p.item));
  }, []);

  /** Takes everything out of the queue first, so a re-entrant call never commits twice. */
  const commitAll = useCallback((): void => {
    const due = pending.current;
    pending.current = [];
    for (const entry of due) entry.commit();
  }, []);

  const flush = useCallback((): void => {
    if (pending.current.length === 0) return;
    commitAll();
    publish();
  }, [commitAll, publish]);

  const remove = useCallback(
    (item: T, commit: () => void): void => {
      commitAll();
      pending.current = [{ item, commit }];
      publish();
    },
    [commitAll, publish],
  );

  const undo = useCallback((): T[] | undefined => {
    const entry = pending.current[pending.current.length - 1];
    if (entry === undefined) return undefined;
    pending.current = pending.current.filter((p) => p !== entry);
    publish();
    return [entry.item];
  }, [publish]);

  useEffect(() => {
    window.addEventListener("pagehide", commitAll);
    return () => {
      window.removeEventListener("pagehide", commitAll);
      commitAll();
    };
  }, [commitAll]);

  return { latest: items[items.length - 1], pending: items, remove, undo, dismiss: flush, flush };
}
