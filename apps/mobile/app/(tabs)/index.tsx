import { type ReactNode } from "react";
import { ScreenScroll } from "@/src/components/ScreenScroll";
import { HomeScreen } from "@/src/screens/HomeScreen";

/** Inicio tab. The home summary is always available (no module flag). */
export default function InicioRoute(): ReactNode {
  return (
    <ScreenScroll>
      <HomeScreen />
    </ScreenScroll>
  );
}
