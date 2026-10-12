import { useFlag, type FlagKey } from "@garcia-lorca/ui/flags";

import { useFlagsResolved } from "../flags-resolved";
import { isRefreshLookShown } from "../look/look";

/**
 * The ONE place the home variant reads its flag (PLAN-redesign-home §3.3,
 * E-flag). The skin reads the same flag in its own single place
 * (look/LookAttribute.tsx, SKIN_FLAG), so splitting it in two later is a
 * one-line change here.
 */
const HOME_FLAG: FlagKey = "module_ui_refresh";

/**
 * True when the refreshed home ("option C") replaces today's. Until /features
 * answers, the flag is the OFF fallback while the skin is the remembered look
 * (pre-paint); the home follows that look instead, so skin and home agree and
 * the home does not swap (re-firing its reads) when the answer lands — or
 * stay mismatched all session if the gateway never answers (review I1).
 */
export function useRefreshedHome(): boolean {
  const flagOn = useFlag(HOME_FLAG);
  const resolved = useFlagsResolved();
  return resolved ? flagOn : isRefreshLookShown();
}
