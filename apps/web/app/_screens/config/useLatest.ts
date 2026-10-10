import { useLayoutEffect, useRef, type RefObject } from "react";

/**
 * A ref that always holds the latest value: a callback prop can be read from a
 * promise's `then` without becoming an effect dependency (a new identity on each
 * render must not trigger a second read).
 */
export function useLatest<T>(value: T): RefObject<T> {
  const ref = useRef(value);
  useLayoutEffect(() => {
    ref.current = value;
  });
  return ref;
}
