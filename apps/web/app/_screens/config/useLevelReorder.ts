"use client";

import { useCallback, useRef, type Dispatch, type SetStateAction } from "react";

import { API_URL } from "../../api-url";
import { reorderLevels, type Level } from "../../config/api";

export interface LevelReorder {
  /** Takes a list the server just sent as the confirmed order, and shows it. */
  adopt: (levels: Level[]) => void;
  /**
   * Puts the levels in this order (every id, first first). The list moves at
   * once; a refusal puts back the order the server last confirmed and rejects.
   */
  reorder: (levelIds: string[]) => Promise<void>;
}

/** `levels` in `order`; any level the order does not name keeps its place at the end. */
function inOrder(levels: readonly Level[], order: readonly string[]): Level[] {
  const rank = new Map(order.map((id, i) => [id, i]));
  const unranked = order.length;
  return [...levels].sort((a, b) => (rank.get(a.id) ?? unranked) - (rank.get(b.id) ?? unranked));
}

/**
 * Quick Subir/Bajar moves, one request at a time.
 *
 * Every move sends the WHOLE order, built on the moves before it. Sent in
 * parallel, the server could answer (or apply) an older one last and the later
 * move would be lost without a word, so the requests form a queue: each waits
 * for the previous answer. Only the newest move's answer is drawn — an older
 * one would pull the list back under the person's hands. A refusal drops the
 * queued moves (they were built on the refused one) and goes back to the order
 * the server last CONFIRMED, never to a snapshot that was itself optimistic.
 */
export function useLevelReorder(setLevels: Dispatch<SetStateAction<Level[]>>): LevelReorder {
  const queue = useRef<Promise<unknown>>(Promise.resolve());
  const confirmedOrder = useRef<string[]>([]);
  const newestRequest = useRef(0);
  const generation = useRef(0);

  const adopt = useCallback((levels: Level[]): void => {
    confirmedOrder.current = levels.map((l) => l.id);
    setLevels(levels);
  }, [setLevels]);

  const reorder = useCallback((levelIds: string[]): Promise<void> => {
    const request = ++newestRequest.current;
    const builtOn = generation.current;
    setLevels((prev) => inOrder(prev, levelIds));

    const send = async (): Promise<void> => {
      if (builtOn !== generation.current) return;
      try {
        const saved = await reorderLevels(API_URL, levelIds);
        confirmedOrder.current = saved.length > 0 ? saved.map((l) => l.id) : levelIds;
        if (request === newestRequest.current && saved.length > 0) setLevels(saved);
      } catch (cause) {
        generation.current += 1;
        setLevels((prev) => inOrder(prev, confirmedOrder.current));
        throw cause;
      }
    };
    const sent = queue.current.then(send);
    queue.current = sent.catch(() => undefined);
    return sent;
  }, [setLevels]);

  return { adopt, reorder };
}
