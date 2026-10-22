import { FeatureGate, type FlagKey } from "@garcia-lorca/ui/flags";
import { type ReactElement, type ReactNode } from "react";
import { ChildScreenScroll } from "./ChildScreenScroll";
import { EmptyState } from "../ui";

export interface GatedScreenProps {
  flag: FlagKey;
  children: ReactNode;
  refreshable?: boolean;
}

/**
 * Wraps a tab screen body in the standard (per-child) scroll container and a UX-only flag
 * gate. When the module flag is OFF the screen shows a neutral "unavailable"
 * state instead of the content (the backend gateway is the real boundary).
 */
export function GatedScreen({ flag, children, refreshable = true }: GatedScreenProps): ReactElement {
  return (
    <ChildScreenScroll refreshable={refreshable}>
      <FeatureGate
        flag={flag}
        fallback={
          <EmptyState
            icon="lock"
            title="Módulo no disponible"
            subtitle="Esta sección no está habilitada para tu cuenta."
          />
        }
      >
        {children}
      </FeatureGate>
    </ChildScreenScroll>
  );
}
