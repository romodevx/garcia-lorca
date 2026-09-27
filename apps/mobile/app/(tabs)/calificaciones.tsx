import { useLocalSearchParams } from "expo-router";
import { type ReactNode } from "react";
import { GatedScreen } from "@/src/components/GatedScreen";
import { GradesScreen } from "@/src/screens/GradesScreen";

/**
 * Calificaciones tab — gated by `module_grades` (UX only). `?vista=boletas`
 * opens the Boletas segment (a boleta push); `k` makes a repeated request apply.
 */
export default function CalificacionesRoute(): ReactNode {
  const { vista, k } = useLocalSearchParams<{ vista?: string; k?: string }>();
  return (
    <GatedScreen flag="module_grades">
      <GradesScreen requestedView={vista === "boletas" ? "boletas" : undefined} requestKey={k} />
    </GatedScreen>
  );
}
