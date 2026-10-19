import { render, type RenderResult } from "@testing-library/react";
import type { FlagSet } from "@garcia-lorca/ui/flags";
import type { ReactElement, ReactNode } from "react";

import { MODULES } from "../../_design/data";
import { Providers } from "../../test-utils";

/** Every module's flag ON: a home under test shows what the permissions allow. */
export const ALL_MODULES_ON: FlagSet = Object.fromEntries(MODULES.map((m) => [m.flag, true]));

/** The two homes the flag chooses between; suites that cover the contract run on both. */
export const HOME_VARIANTS = [
  ["classic", false],
  ["refresh", true],
] as const;

/**
 * Renders a home under the app providers, with module_ui_refresh set as asked.
 * Every module flag is ON unless flags says otherwise.
 */
export function renderHome(ui: ReactElement, refresh: boolean, flags: FlagSet = ALL_MODULES_ON): RenderResult {
  const wrapper = ({ children }: { children: ReactNode }): ReactElement => (
    <Providers flags={{ ...flags, module_ui_refresh: refresh }}>{children}</Providers>
  );
  return render(ui, { wrapper });
}
