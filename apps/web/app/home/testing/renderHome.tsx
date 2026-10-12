import { render, type RenderResult } from "@testing-library/react";
import type { FlagSet } from "@garcia-lorca/ui/flags";
import type { ReactElement, ReactNode } from "react";

import { Providers } from "../../test-utils";

/** The two homes the flag chooses between; suites that cover the contract run on both. */
export const HOME_VARIANTS = [
  ["classic", false],
  ["refresh", true],
] as const;

/** Renders a home under the app providers, with module_ui_refresh set as asked. */
export function renderHome(ui: ReactElement, refresh: boolean, flags: FlagSet = {}): RenderResult {
  const wrapper = ({ children }: { children: ReactNode }): ReactElement => (
    <Providers flags={{ ...flags, module_ui_refresh: refresh }}>{children}</Providers>
  );
  return render(ui, { wrapper });
}
