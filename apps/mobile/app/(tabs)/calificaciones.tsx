import { type ReactNode } from "react";
import { GatedScreen } from "@/src/components/GatedScreen";
import { GradesScreen } from "@/src/screens/GradesScreen";

/** Calificaciones tab — gated by `module_grades` (UX only). */
export default function CalificacionesRoute(): ReactNode {
  return (
    <GatedScreen flag="module_grades">
      <GradesScreen />
    </GatedScreen>
  );
}
