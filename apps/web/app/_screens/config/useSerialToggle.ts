import { useCallback, useRef, useState } from "react";

export interface SerialToggle {
  /** What the switch shows: the latest wish, optimistically. */
  on: boolean;
  toggle: (next: boolean) => void;
}

/**
 * An optimistic switch whose saves never overlap: at most one request is in
 * flight; clicks made meanwhile only update the wish, and when the request
 * settles the latest wish is sent if it still differs from what the server
 * holds. A failed save puts the switch back where the server is.
 */
export function useSerialToggle(initial: boolean, save: (next: boolean) => Promise<boolean>): SerialToggle {
  const [on, setOn] = useState(initial);
  const confirmed = useRef(initial);
  const wanted = useRef(initial);
  const inFlight = useRef(false);

  const pump = useCallback(async (): Promise<void> => {
    inFlight.current = true;
    while (wanted.current !== confirmed.current) {
      const target = wanted.current;
      if (!(await save(target))) {
        wanted.current = confirmed.current;
        setOn(confirmed.current);
        break;
      }
      confirmed.current = target;
    }
    inFlight.current = false;
  }, [save]);

  const toggle = useCallback((next: boolean) => {
    wanted.current = next;
    setOn(next);
    if (!inFlight.current) void pump();
  }, [pump]);

  return { on, toggle };
}
