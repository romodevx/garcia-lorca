"use client";

import { useCallback, useState } from "react";

import { emailShapeError } from "./emailShape";
import { useFocusFirstInvalid } from "./useFocusFirstInvalid";

export interface EmailShapeOnSave {
  /** The field's error, to pass as EmailField's `error`. */
  readonly error: string | undefined;
  /**
   * On SAVE: judges the address; if it is wrong, marks the field, focuses it
   * and returns false. Needed because Enter (or a keyboard-activated button)
   * saves without the field ever losing focus, so EmailField's on-blur check
   * never ran. An empty address passes — whether it may be empty is the form's
   * call.
   */
  readonly check: (value: string) => boolean;
  /** On each keystroke: clears the red only once the address is right. */
  readonly forgive: (value: string) => void;
}

/**
 * The email-shape rule at save time, shared by every form that captures an
 * address. It accuses on save and forgives per keystroke — never the other way.
 */
export function useEmailShapeOnSave(): EmailShapeOnSave {
  const [error, setError] = useState<string | undefined>(undefined);
  const focusFirstInvalid = useFocusFirstInvalid();

  const check = useCallback(
    (value: string): boolean => {
      const found = emailShapeError(value);
      setError(found);
      if (found !== undefined) focusFirstInvalid();
      return found === undefined;
    },
    [focusFirstInvalid],
  );

  const forgive = useCallback((value: string): void => {
    if (emailShapeError(value) === undefined) setError(undefined);
  }, []);

  return { error, check, forgive };
}
