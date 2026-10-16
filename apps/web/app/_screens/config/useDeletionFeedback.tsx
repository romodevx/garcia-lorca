"use client";

import { useCallback, useEffect, useRef, useState, type CSSProperties, type ReactElement, type RefObject } from "react";

const visuallyHidden: CSSProperties = { position: "absolute", width: 1, height: 1, overflow: "hidden", clip: "rect(0 0 0 0)", whiteSpace: "nowrap" };

interface Deletion {
  message: string;
  target: HTMLElement | null;
}

export interface DeletionFeedback {
  /** Says `message` and moves the focus to `target`, or to the create control when there is none. */
  deleted: (message: string, target: HTMLElement | null) => void;
  /** Put on the box that holds the section's create control: the last resort for the focus. */
  fallbackRef: RefObject<HTMLDivElement | null>;
  message: string;
}

/**
 * After a row is deleted it is gone, and so is the button that had the focus:
 * left alone the focus falls to <body>. This puts it on the place the caller
 * picked (the next or previous row) or, when no row is left, on the section's
 * create control — from an effect, so it runs after the row and the dialog have
 * left the DOM and nothing they do on the way out can take it back.
 */
export function useDeletionFeedback(): DeletionFeedback {
  const [deletion, setDeletion] = useState<Deletion | undefined>(undefined);
  const fallbackRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (deletion === undefined) return;
    const target = deletion.target?.isConnected === true ? deletion.target : fallbackRef.current?.querySelector<HTMLElement>("button, input");
    target?.focus();
  }, [deletion]);

  const deleted = useCallback((message: string, target: HTMLElement | null): void => setDeletion({ message, target }), []);
  return { deleted, fallbackRef, message: deletion?.message ?? "" };
}

/** The polite region that says what was just deleted. Always mounted, so the first message is heard. */
export function DeletionStatus({ message }: { message: string }): ReactElement {
  return <p role="status" style={visuallyHidden}>{message}</p>;
}
