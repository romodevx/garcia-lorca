"use client";

import { useSyncExternalStore, type ReactNode } from "react";
import { createPortal } from "react-dom";

const subscribeNever = (): (() => void) => () => {};
const onClient = (): boolean => true;
const onServer = (): boolean => false;

/**
 * Renders its children straight into `document.body`.
 *
 * For overlays that must not depend on where they are drawn: inside a `hidden`
 * surface they would vanish, and inside a transformed ancestor `position: fixed`
 * is fixed to that ancestor, not to the viewport. On the server (and during
 * hydration) there is no `document`, so nothing is rendered until the client
 * takes over — the same first paint on both sides, no hydration mismatch.
 */
export function BodyPortal({ children }: { children: ReactNode }): ReactNode {
  const mounted = useSyncExternalStore(subscribeNever, onClient, onServer);
  return mounted ? createPortal(children, document.body) : null;
}
