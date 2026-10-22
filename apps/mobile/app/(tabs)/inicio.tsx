import { type ReactNode } from "react";
import { ChildScreenScroll } from "@/src/components/ChildScreenScroll";
import { HomeScreen } from "@/src/screens/HomeScreen";

/** Inicio tab. The home summary is always available (no module flag). */
export default function InicioRoute(): ReactNode {
  return (
    <ChildScreenScroll>
      <HomeScreen />
    </ChildScreenScroll>
  );
}
