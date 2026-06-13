import { render, type RenderResult } from "@testing-library/react-native";
import { type ReactElement } from "react";
import { Providers, type ProvidersProps } from "../Providers";

export type RenderOptions = Omit<ProvidersProps, "children">;

/**
 * Renders a component inside the full app provider stack (theme, flags, app
 * state, toast, safe area). RNTL v14 render is async, so callers must await.
 */
export function renderWithProviders(ui: ReactElement, options: RenderOptions = {}): Promise<RenderResult> {
  return render(<Providers {...options}>{ui}</Providers>);
}
