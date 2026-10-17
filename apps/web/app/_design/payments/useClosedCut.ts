import { useEffect, useState } from "react";

import { API_URL } from "../../api-url";
import { getCashUp } from "../../finance/api";

/**
 * Whether the cash cut of the day a payment lands on is already closed
 * (ISSUE-026). Collecting then is allowed — it only marks the cut "modificado
 * después del cierre" — so this feeds a warning, never a block. `day` is ""
 * for today (the server's own day). A cut that cannot be read (no permission,
 * the network) reads as open: the counter keeps working as before.
 */
export function useClosedCut(day: string, enabled: boolean): boolean {
  const [closed, setClosed] = useState(false);

  useEffect(() => {
    setClosed(false);
    if (!enabled) return;
    let live = true;
    void (async () => {
      try {
        const cut = await getCashUp(API_URL, day);
        if (live) setClosed(cut.closure !== undefined);
      } catch {
        if (live) setClosed(false);
      }
    })();
    return () => {
      live = false;
    };
  }, [day, enabled]);

  return closed;
}
