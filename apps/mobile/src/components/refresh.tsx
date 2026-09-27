import { createContext, useContext, useEffect, useRef } from "react";

/**
 * Lets a screen's content tell the scroll container around it what a pull
 * should reload. The route owns the <ScreenScroll>, the screen owns the data
 * hooks; this bridge keeps both where they belong.
 */

export type RefreshHandler = () => Promise<void>;

/** Registers a handler and returns its unregister function. */
export type RegisterRefresh = (handler: RefreshHandler) => () => void;

export const RefreshRegistryContext = createContext<RegisterRefresh | null>(null);

/**
 * Makes `handler` part of the surrounding pull-to-refresh. Outside a
 * ScreenScroll it is a no-op, so a screen renders the same in tests and in a
 * container that does not refresh.
 */
export function useRefreshHandler(handler: RefreshHandler): void {
  const register = useContext(RefreshRegistryContext);
  const latest = useRef(handler);
  latest.current = handler;
  useEffect(() => {
    if (register === null) {
      return undefined;
    }
    return register(() => latest.current());
  }, [register]);
}
