import { useCallback, useRef, useState } from "react";

import type { LevelGrading } from "../../config/api";

/**
 * Saves one level's grading. `confirmed` is what the server held before this
 * request (to put a refused scale back); resolves true when the server took it.
 */
export type SaveGrading = (target: LevelGrading, confirmed: LevelGrading) => Promise<boolean>;

export interface SerialGrading {
  /** What the switch shows: the latest wish, optimistically. */
  enabled: boolean;
  toggle: (next: boolean) => void;
  /** Queue a scale save; it goes out after any request in flight. */
  saveScale: (scale: LevelGrading) => void;
}

/**
 * One queue for everything a level's grading saves — the switch and the scale
 * form — so no two requests overlap and neither overwrites the other with a
 * stale copy. At most one request is in flight. Toggles made meanwhile only
 * update the wish; a scale saved meanwhile waits (the latest one wins). When a
 * request settles, the queued scale goes out (with the switch's current wish),
 * then the switch's wish if it still differs from what the server holds — built
 * from the last CONFIRMED scale, never a half-typed one. A failure puts the
 * switch back where the server is and drops what was queued behind it.
 */
export function useSerialGrading(initial: LevelGrading, save: SaveGrading): SerialGrading {
  const [enabled, setEnabled] = useState(initial.enabled);
  const confirmed = useRef<LevelGrading>(initial);
  const wantedEnabled = useRef(initial.enabled);
  const queuedScale = useRef<LevelGrading | null>(null);
  const inFlight = useRef(false);

  const nextTarget = useCallback((): LevelGrading | null => {
    const scale = queuedScale.current;
    queuedScale.current = null;
    if (scale !== null) return { ...scale, enabled: wantedEnabled.current };
    if (wantedEnabled.current !== confirmed.current.enabled) return { ...confirmed.current, enabled: wantedEnabled.current };
    return null;
  }, []);

  const pump = useCallback(async (): Promise<void> => {
    inFlight.current = true;
    for (let target = nextTarget(); target !== null; target = nextTarget()) {
      if (!(await save(target, confirmed.current))) {
        queuedScale.current = null;
        wantedEnabled.current = confirmed.current.enabled;
        setEnabled(confirmed.current.enabled);
        break;
      }
      confirmed.current = target;
    }
    inFlight.current = false;
  }, [nextTarget, save]);

  const toggle = useCallback((next: boolean) => {
    wantedEnabled.current = next;
    setEnabled(next);
    if (!inFlight.current) void pump();
  }, [pump]);

  const saveScale = useCallback((scale: LevelGrading) => {
    queuedScale.current = scale;
    if (!inFlight.current) void pump();
  }, [pump]);

  return { enabled, toggle, saveScale };
}
