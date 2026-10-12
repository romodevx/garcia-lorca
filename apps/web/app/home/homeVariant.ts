import { useFlag, type FlagKey } from "@garcia-lorca/ui/flags";

/**
 * The ONE place the home variant reads its flag (PLAN-redesign-home §3.3,
 * E-flag). The skin reads the same flag in its own single place
 * (look/LookAttribute.tsx, SKIN_FLAG), so splitting it in two later is a
 * one-line change here.
 */
const HOME_FLAG: FlagKey = "module_ui_refresh";

/** True when the refreshed home ("option C") replaces today's. */
export function useRefreshedHome(): boolean {
  return useFlag(HOME_FLAG);
}
