import { type ReactNode } from "react";
import { GatedScreen } from "@/src/components/GatedScreen";
import { AttendanceScreen } from "@/src/screens/AttendanceScreen";

/** Asistencia tab — gated by `module_attendance` (UX only). */
export default function AsistenciaRoute(): ReactNode {
  return (
    <GatedScreen flag="module_attendance">
      <AttendanceScreen />
    </GatedScreen>
  );
}
