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
  /** What the letters switch shows (only meaningful while numbers are off). */
  letters: boolean;
  toggleLetters: (next: boolean) => void;
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
 * switch back where the server is and drops what was queued behind it. The
 * letters switch shares the queue: turning numbers on turns letters off.
 */
export function useSerialGrading(initial: LevelGrading, save: SaveGrading): SerialGrading {
  const start: LevelGrading = { ...initial, letters: !initial.enabled && initial.letters === true };
  const [enabled, setEnabled] = useState(start.enabled);
  const [letters, setLetters] = useState(start.letters === true);
  const confirmed = useRef<LevelGrading>(start);
  const wantedEnabled = useRef(start.enabled);
  const wantedLetters = useRef(start.letters === true);
  const queuedScale = useRef<LevelGrading | null>(null);
  const inFlight = useRef(false);

  const nextTarget = useCallback((): LevelGrading | null => {
    const scale = queuedScale.current;
    queuedScale.current = null;
    // Numbers win: a level that grades with numbers never grades with letters.
    const wish = { enabled: wantedEnabled.current, letters: !wantedEnabled.current && wantedLetters.current };
    if (scale !== null) return { ...scale, ...wish };
    if (wish.enabled !== confirmed.current.enabled || wish.letters !== (confirmed.current.letters === true)) {
      return { ...confirmed.current, ...wish };
    }
    return null;
  }, []);

  const pump = useCallback(async (): Promise<void> => {
    inFlight.current = true;
    for (let target = nextTarget(); target !== null; target = nextTarget()) {
      if (!(await save(target, confirmed.current))) {
        queuedScale.current = null;
        wantedEnabled.current = confirmed.current.enabled;
        wantedLetters.current = confirmed.current.letters === true;
        setEnabled(confirmed.current.enabled);
        setLetters(wantedLetters.current);
        break;
      }
      confirmed.current = target;
    }
    inFlight.current = false;
  }, [nextTarget, save]);

  const toggle = useCallback((next: boolean) => {
    wantedEnabled.current = next;
    setEnabled(next);
    if (next) {
      wantedLetters.current = false;
      setLetters(false);
    }
    if (!inFlight.current) void pump();
  }, [pump]);

  const toggleLetters = useCallback((next: boolean) => {
    wantedLetters.current = next;
    setLetters(next);
    if (!inFlight.current) void pump();
  }, [pump]);

  const saveScale = useCallback((scale: LevelGrading) => {
    queuedScale.current = scale;
    if (!inFlight.current) void pump();
  }, [pump]);

  return { enabled, toggle, letters, toggleLetters, saveScale };
}
