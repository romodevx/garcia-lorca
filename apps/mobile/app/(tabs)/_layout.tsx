import { Tabs } from "expo-router";
import type { ReactNode } from "react";

/**
 * Parent/student bottom-tab navigation. FASE 0 stub: tabs are declared but each
 * screen is an empty placeholder. Per-module tabs must be gated (FeatureGate /
 * useFlag) and their screens lazy-loaded so disabled module code never ships.
 */
export default function TabsLayout(): ReactNode {
  return (
    <Tabs screenOptions={{ headerShown: true }}>
      <Tabs.Screen name="asistencia" options={{ title: "Asistencia" }} />
      <Tabs.Screen name="calificaciones" options={{ title: "Calificaciones" }} />
      <Tabs.Screen name="estado-cuenta" options={{ title: "Estado de cuenta" }} />
    </Tabs>
  );
}
